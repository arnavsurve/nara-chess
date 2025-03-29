package types

type GameStateRequest struct {
	MoveHistory []string `json:"move_history"`
	Fen         string   `json:"fen"`
	WrongMove   string   `json:"wrong_move"`
}

type GameStateResponse struct {
	Comment string `json:"comment"`
	Move    string `json:"move"`
}
