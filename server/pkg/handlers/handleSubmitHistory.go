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
	var wrongMove string
	if gameStateRequest.WrongMove != "" {
		wrongMove = fmt.Sprintf("\n\nHere, %s is an INVALID MOVE. Do not use this in your response.", gameStateRequest.WrongMove)
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

	model := client.GenerativeModel("gemini-2.0-flash")

	gameStateResponseSchema := &genai.Schema{
		Type:        genai.TypeObject,
		Description: "Response containing commentary on the chess game state and next move.",
		Properties: map[string]*genai.Schema{
			"comment": {
				Type:        genai.TypeString,
				Description: "A brief commentary (1-3 sentences) on the current game situation, evaluating the state of the game for black and white. Include coaching information here.",
			},
			"move": {
				Type:        genai.TypeString,
				Description: "The move you would like to make in Standard Algebraic Notation (SAN), e.g., 'Nf3', 'O-O', 'e8=Q+'.",
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

	llmSide, pupilSide, err := inferSidesFromFEN(gameStateRequest.Fen)
	if err != nil {
		log.Printf("Error parsing FEN for side inference: %v", err)
		http.Error(w, "Invalid FEN", http.StatusBadRequest)
	}

	promptText := fmt.Sprintf(`You are a strong chess engine, commentator, and coach playing a friendly educational match against your favorite pupil.

You are playing as %s.
Your pupil is playing as %s.
The current position is after your pupil just made their move — it is now your turn.

Your job is to:
1. Make the best move for your side (%s) based on the FEN.
2. Give your pupil (the %s player) coaching and insight on the position.
3. Explain your move in a way that helps them learn and improve.

Refer to your pupil using second person pronouns ("you", "your").  
Refer to yourself only in the first person ("I", "my").  
Never refer to both yourself and your pupil using collective terms like "we", "us", or "our".  
Do not use inclusive language like "let's", "we're", or "our plan".  
You are the opponent and coach — your pupil is the one learning by playing against you.  
Maintain a clear distinction between your actions and your pupil's actions at all times.

FEN: %s
Move History: %s

Strictly format your response as a JSON object matching this schema:
{
  "comment": "...", // A brief 1-3 sentence comment from you to your pupil
  "move": "..."     // Your move in SAN (e.g., "Nf3", "O-O", "e8=Q+")
}


Do NOT include any explanation or extra text outside the JSON object.

JSON object required:`, llmSide, pupilSide, llmSide, pupilSide, gameStateRequest.Fen, moveHistoryStr)

	prompt := genai.Text(promptText + wrongMove)

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

func inferSidesFromFEN(fen string) (llmSide string, pupilSide string, err error) {
	parts := strings.Split(fen, " ")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid FEN: not enough parts")
	}
	turn := parts[1]
	switch turn {
	case "w":
		return "White", "Black", nil // White to move, so Black was the pupil
	case "b":
		return "Black", "White", nil // Black to move, so White was the pupil
	default:
		return "", "", fmt.Errorf("invalid FEN turn field: %s", turn)
	}
}
