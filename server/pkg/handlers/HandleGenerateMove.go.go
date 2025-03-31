package handlers

import (
	"arnavsurve/nara-chess/server/pkg/types"
	"arnavsurve/nara-chess/server/pkg/utils"
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

func HandleGenerateMove(w http.ResponseWriter, r *http.Request) {
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

	model := client.GenerativeModel("gemini-2.5-pro-exp-03-25")

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
			"arrows": {
				Type:        genai.TypeArray,
				Description: "Optional coaching arrows to display. Each is a tuple of two square strings (from, to). Used to show threats, good ideas, plans, etc.",
				Items: &genai.Schema{
					Type: genai.TypeArray,
					Items: &genai.Schema{
						Type: genai.TypeString,
					},
				},
			},
			"title": {
				Type:        genai.TypeString,
				Description: "A short phrase to describe the current game.",
			},
		},
		Required: []string{"comment", "move"},
	}

	model.GenerationConfig = genai.GenerationConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   gameStateResponseSchema,
		Temperature:      utils.PtrFloat32(0.4),
	}

	moveHistoryStr := strings.Join(gameStateRequest.MoveHistory, " ")

	llmSide, pupilSide, err := utils.InferSidesFromFEN(gameStateRequest.Fen)
	if err != nil {
		log.Printf("Error parsing FEN for side inference: %v", err)
		http.Error(w, "Invalid FEN", http.StatusBadRequest)
	}

	promptText := fmt.Sprintf(`You are a strong chess engine, commentator, and coach in an ongoing educational match against your pupil.

You are playing as %s.  
Your pupil is playing as %s.  
It is currently your turn to move — your pupil just made the last move.  

You must:
1. Select the best next move for your side (%s) using strong chess principles.
2. Evaluate the position for both sides — from your pupil’s perspective.
3. Provide insightful, constructive feedback that helps your pupil improve.

In your response:
- Identify specific positional features (e.g., weak squares, piece activity, king safety, space, pawn structure).
- **Explain the ideas behind your move and how it fits into a short-term or long-term plan.**
- Mention any **good ideas** or **mistakes** your pupil made in their last move or overall game direction.
- **Offer a brief tactical or strategic concept they could focus on (e.g., "look for pins", "consider open files", "avoid weakening squares like f3").**
- **Relate their move to classical principles or named openings if appropriate (e.g., “this is common in the Italian Game”)**.
- Use clear and simple language and talk in a casual tone, minimizing filler language. Be direct in your communication.
- Think deeply when formulating your response to provide appropriate coaching based on the opponent's estimated skill level and bringing up interesting lines or characteristics of the game state.

- If useful, include a list of 1–3 arrows that would help the pupil visualize the plan, threats, or key ideas on the board. ENSURE YOU ELABORATE ON THE MOVES THAT THESE ARROWS DESCRIBE. Only use arrows to help illustrate your description of *future moves*, threats, or key ideas. Do not use arrows without already having described the scenario for that arrow. Do not use an arrow to indicate a move that you or the player has made already or is currently making.
- Use the format: ["from-square", "to-square"] — for example: ["e4", "e5"] to suggest a pawn push.
- These arrows are used to help the user *learn*, so show things like threats, weak squares, tactical ideas, or developing moves that may be applicable to either side.
- DO NOT use arrows unless the game's position ABSOLUTELY NECESSITATES an opportunity for in depth analysis. For textbook positions or early game, DO NOT RETURN ANY ARROWS.


**Pronoun usage rules**:
- Refer to yourself as “I” and to the pupil as “you”.
- Do **not** use “we”, “us”, or “our”.

FEN: %s  
Move History: %s

Output your response **strictly** as a JSON object matching this schema:

{
  "comment": "...", // Constructive coaching commentary (1–3 sentences)
  "move": "..."     // Your move in SAN (e.g., "Nf3", "O-O", "e8=Q+")
  "arrows": [["e4", "e5"], ["g1", "f3"]]
  "title": "Italian Game, Hectic Endgame, King's Gambit, Unique Opening"
}

Do NOT include anything outside the JSON object.`, llmSide, pupilSide, llmSide, gameStateRequest.Fen, moveHistoryStr)

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
