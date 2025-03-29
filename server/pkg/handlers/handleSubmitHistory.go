package handlers

import (
	"arnavsurve/nara-chess/server/pkg/types"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

func HandleSubmitHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var gameStateRequest types.GameStateRequest

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // Limit body size to 1MB

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	err := decoder.Decode(&gameStateRequest)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if len(gameStateRequest.MoveHistory) == 0 && gameStateRequest.Fen == "" {
		http.Error(w, "Request must contain either move_history or fen", http.StatusBadRequest)
		return
	}
	if gameStateRequest.Fen == "" {
		http.Error(w, "Request must contain the current board state FEN (fen field)", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // 60 second timeout
	defer cancel()

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Println("ERROR: GEMINI_API_KEY environment variable not set.")
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		log.Printf("Error creating Gemini client: %v", err)
		http.Error(w, "Failed to initialize analysis service", http.StatusInternalServerError)
		return
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.5-pro-exp-03-25")

	gameStateResponseSchema := &genai.Schema{
		Type:        genai.TypeObject,
		Description: "Response containing commentary on the chess game state and the suggested next best move.",
		Properties: map[string]*genai.Schema{
			"comment": {
				Type:        genai.TypeString,
				Description: "A brief commentary (1-3 sentences) on the current game situation, evaluating the state of the game for black and white. Include coaching information here.",
			},
			"move": {
				Type:        genai.TypeString,
				Description: "The suggested best next move for the current player in Standard Algebraic Notation (SAN), e.g., 'Nf3', 'O-O', 'e8=Q+'.",
			},
		},
		Required: []string{"comment", "move"},
	}

	model.GenerationConfig = genai.GenerationConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   gameStateResponseSchema,
		Temperature:      ptrFloat32(0.4),
	}

	moveHistoryStr := strings.Join(gameStateRequest.MoveHistory, " ")

	promptText := fmt.Sprintf(`You are a strong chess engine commentator, and coach.
Analyze the following chess position, provided by the FEN string and the preceding move history.
Determine the best move for the player whose turn it currently is (as indicated by the FEN).
Provide a brief commentary (1-3 sentences) on the current state of the game from the perspective of the player to move. Include constructive coaching in your commentary.

Current FEN: %s
Move History leading to this position: %s

Instructions:
1. Identify the best legal move in this position according to strong chess principles.
2. Provide a comment evaluating the current situation (e.g., material balance, king safety, activity, space).
3. Format your response *strictly* as a JSON object matching the provided schema. Only output the JSON object, with no introductory text or explanation before or after it.
   - The "comment" field should contain your commentary.
   - The "move" field should contain *only* the suggested best move in Standard Algebraic Notation (SAN). Examples: "e4", "Nf3", "Qxb7", "O-O", "fxg1=Q+".

JSON object required:`, gameStateRequest.Fen, moveHistoryStr)

	prompt := genai.Text(promptText)

	log.Printf("Sending request to Gemini for move suggestion. FEN: %s", gameStateRequest.Fen)
	resp, err := model.GenerateContent(ctx, prompt)
	if err != nil {
		log.Printf("Error generating content from Gemini: %v", err)
		if errors.Is(err, context.DeadlineExceeded) {
			http.Error(w, "Analysis request timed out", http.StatusGatewayTimeout)
		} else {
			http.Error(w, "Failed to get move suggestion from service", http.StatusInternalServerError)
		}
		return
	}

	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		log.Printf("Error: Received empty or invalid response structure from Gemini. Response: %+v", resp)
		http.Error(w, "Received empty analysis response", http.StatusInternalServerError)
		return
	}

	jsonPart := resp.Candidates[0].Content.Parts[0]
	jsonString, ok := jsonPart.(genai.Text)
	if !ok {
		log.Printf("Error: Expected response part to be genai.Text, but got %T. Content: %+v", jsonPart, jsonPart)
		http.Error(w, "Received unexpected analysis format from service", http.StatusInternalServerError)
		return
	}

	log.Printf("Raw JSON received from Gemini: %s", jsonString)

	var gameStateResponse types.GameStateResponse
	err = json.Unmarshal([]byte(jsonString), &gameStateResponse)
	if err != nil {
		log.Printf("Error unmarshalling Gemini JSON response: %v\nRaw JSON was: %s", err, jsonString)
		http.Error(w, "Failed to parse move suggestion", http.StatusInternalServerError)
		return
	}

	if gameStateResponse.Move == "" {
		log.Printf("Warning: Gemini returned JSON but the 'move' field was empty. Raw: %s", jsonString)
		// Decide how to handle this - maybe retry, maybe return an error?
		// For now, we'll let it pass but log it. Could return error:
		http.Error(w, "Analysis service failed to provide a move", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(gameStateResponse)
	if err != nil {
		log.Printf("Error encoding JSON response for client: %v", err)
	}

	log.Printf("Successfully processed request. Suggested move: %s", gameStateResponse.Move)
}

func ptrFloat32(f float32) *float32 {
	return &f
}
