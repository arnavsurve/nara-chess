package utils

import (
	"fmt"
	"strings"
)

func PtrFloat32(f float32) *float32 {
	return &f
}

func InferSidesFromFEN(fen string) (llmSide string, pupilSide string, err error) {
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
