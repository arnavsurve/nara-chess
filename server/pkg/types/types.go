package types

type ChatMessage struct {
	Content string `json:"content"`
	Role    string `json:"role"`
}

type GameStateRequest struct {
	MoveHistory []string      `json:"move_history"`
	ChatHistory []ChatMessage `json:"chat_history"`
	Fen         string        `json:"fen"`
	WrongMove   string        `json:"wrong_move"`
}

type GameStateResponse struct {
	Comment string      `json:"comment"`
	Move    string      `json:"move"`
	Arrows  [][2]string `json:"arrows"`
	Title   string      `json:"title"`
}

type ChatMessageRequest struct {
	MessageHistory []ChatMessage    `json:"message_history"`
	GameState      GameStateRequest `json:"game_state"`
	PlayerSide     string           `json:"player_side"`
}

type ChatMessageResponse struct {
	Response string      `json:"response"`
	Arrows   [][2]string `json:"arrows"`
}
