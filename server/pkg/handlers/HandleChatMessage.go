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

func HandleChatMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var chatMessageRequest types.ChatMessageRequest

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // Limit body size to 1MB

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	err := decoder.Decode(&chatMessageRequest)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	fmt.Println(chatMessageRequest.MessageHistory)

	if chatMessageRequest.GameState.Fen == "" {
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

	chatMessageResponseSchema := &genai.Schema{
		Type:        genai.TypeObject,
		Description: "Response to the user's message.",
		Properties: map[string]*genai.Schema{
			"response": {
				Type:        genai.TypeString,
				Description: "A brief message (1-3 sentences) replying to the user.",
			},
			"arrows": {
				Type:        genai.TypeArray,
				Description: "Optional coaching arrows to display. Each is a tuple of two square strings (from, to). Used to illustrate your response, threats, good ideas, plans, etc.",
				Items: &genai.Schema{
					Type: genai.TypeArray,
					Items: &genai.Schema{
						Type: genai.TypeString,
					},
				},
			},
		},
		Required: []string{"response"},
	}

	model.GenerationConfig = genai.GenerationConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   chatMessageResponseSchema,
		Temperature:      utils.PtrFloat32(0.4),
	}

	moveHistoryStr := strings.Join(chatMessageRequest.GameState.MoveHistory, " ")

	var pupilSide string
	var llmSide string
	if chatMessageRequest.PlayerSide == "white" {
		pupilSide = "white"
		llmSide = "black"
	} else {
		pupilSide = "black"
		llmSide = "white"
	}

	promptText := fmt.Sprintf(`You are a powerful chess coach and engine engaged in an ongoing conversation with your pupil. You are analyzing their game and helping them improve their play, move by move.

You are playing as %s.
Your pupil is playing as %s.

Your goal is to continue the conversation naturally, providing both coaching and analysis. You may respond to the pupil however it may seem fit. The conversation does not have to be strictly about the game.

You are given:
- The current board state in FEN format
- A history of moves made so far
- A transcript of the ongoing chat conversation between you and your pupil

### Your tasks:
1. Continue the conversation by replying **as yourself (the coach)** — include helpful insights, coaching feedback, answers to the pupil's questions, or casual conversation.
2. **Optionally** include a list of up to 3 arrows that help the pupil visualize ideas like threats, tactics, or plans. If you mention any moves in your response relating to any deep analysis, you may include arrows to illustrate these moves.

### Requirements for your response:
- Speak in a friendly, direct tone.
- Stay in character as a helpful coach who explains ideas clearly.
- Use plain English with concrete reasoning and chess terminology.
- Reference positional features (e.g., weak squares, pawn structure, activity, king safety) and classical ideas when relevant.
- ONLY include arrows if they help **illustrate your explanation** or to explain something that your pupil asked. Do NOT use them for already-played moves.
- NEVER say "we" or "us" — refer to yourself as “I” and the pupil as “you”.

### Input
- FEN: %s  
- Move History: %s  
- Chat History (most recent messages last):  
%s

### Response Format
Respond ONLY with a JSON object in the following format:

{
  "response": "...",  // Your chat response and coaching commentary (1–3 sentences or more, continuing the conversation)
  "arrows": [["e4", "e5"], ["g1", "f3"]]  // 0–3 arrows to illustrate your response
}`, llmSide, pupilSide, chatMessageRequest.GameState.Fen, moveHistoryStr, formatChatHistory(chatMessageRequest.MessageHistory))
	fmt.Println(promptText)
	prompt := genai.Text(promptText)

	log.Printf("Sending request to Gemini for move suggestion. FEN: %s", chatMessageRequest.GameState.Fen)
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

	var chatMessageResponse types.ChatMessageResponse
	err = json.Unmarshal([]byte(jsonString), &chatMessageResponse)
	if err != nil {
		log.Printf("Error unmarshalling Gemini JSON response: %v\nRaw JSON was: %s", err, jsonString)
		http.Error(w, "Failed to parse move suggestion", http.StatusInternalServerError)
		return
	}

	if chatMessageResponse.Response == "" {
		log.Printf("Warning: Gemini returned JSON but the 'response' field was empty. Raw: %s", jsonString)
		http.Error(w, "Analysis service failed to provide a response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(chatMessageResponse)
	if err != nil {
		log.Printf("Error encoding JSON response for client: %v", err)
	}

	log.Printf("Successfully processed request. Response: %s", chatMessageResponse.Response)
}

func formatChatHistory(messages []types.ChatMessage) string {
	var sb strings.Builder
	for _, msg := range messages {
		sender := "Pupil"
		if msg.Role == "model" {
			sender = "Coach"
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", sender, msg.Content))
	}
	return sb.String()
}
