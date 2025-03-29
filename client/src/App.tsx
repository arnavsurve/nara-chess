import { useEffect, useState, useRef, SetStateAction, useCallback } from "react";
import { Chessboard } from "react-chessboard";
import { Chess, Move, Square } from "chess.js";
import { ClipLoader } from "react-spinners";
import "./App.css";

interface LLMResponse {
  summary: string;
  move: string;
}

export default function App() {
  const [game, setGame] = useState(new Chess());
  const [moveHistory, setMoveHistory] = useState<string[]>([]);
  const [llmComment, setLLMComment] = useState<string>("");
  const [isPlayerTurn, setIsPlayerTurn] = useState<boolean>(true);
  const [isLoading, setIsLoading] = useState<boolean>(false);

  const makePlayerMove = useCallback((move: Move): boolean => {
    if (!isPlayerTurn || isLoading) return false;

    const gameCopy = new Chess(game.fen());
    try {
      const result = gameCopy.move(move);
      if (result) {
        setGame(gameCopy);
        setMoveHistory(prev => [...prev, result.san]);
        setIsPlayerTurn(false);
        setLLMComment("");
        return true;
      }
    } catch (error) {
      console.warn("Invalid move attempt:", error);
      return false;
    }
    return false;
  }, [game, isPlayerTurn, isLoading]);

  const makeLLMMove = useCallback((llmMoveSan: string) => {
    const gameCopy = new Chess(game.fen());
    try {
      const result = gameCopy.move(llmMoveSan);
      if (result) {
        setGame(gameCopy);
        setMoveHistory(prev => [...prev, result.san]);
        setIsPlayerTurn(true);
        return true;
      } else {
        console.error(`LLM suggested an illegal move: ${llmMoveSan} in FEN: ${game.fen()}`);
        setLLMComment(`Error: The AI suggested an illegal move (${llmMoveSan}). It might be confused! It's your turn.`);
        setIsPlayerTurn(true);
        return false;
      }
    } catch (error) {
      console.error(`Error executing LLM move '${llmMoveSan}':`, error);
      setLLMComment(`Error: Could not execute the AI's move (${llmMoveSan}). It's your turn.`);
      setIsPlayerTurn(true);
      return false;
    }
  }, [game]);

  // Handle piece movement
  function onDrop(sourceSquare: Square, targetSquare: Square) {
    const move = makePlayerMove({
      from: sourceSquare,
      to: targetSquare,
    });

    // illegal move
    if (!move) return false;
    return true;
  }

  useEffect(() => {
    // Only proceed if it's not the player's turn and we're not already loading
    if (isPlayerTurn || isLoading || moveHistory.length < 1) {
      return;
    }

    setIsLoading(true);

    const payload = {
      move_history: moveHistory,
      fen: game.fen()
    }
    console.log("sending to api", payload);

    fetch("http://localhost:42069/submitMove", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    })
      .then((response) => {
        if (!response.ok) {
          throw new Error("Failed to send data to endpoint")
        }
        return response.json()
      })
      .then((data) => {
        console.log("Response from server:", data);
        setLLMComment(data.comment);
        makeLLMMove(data.move);
      })
      .catch((error) => {
        console.error("Error sending data:", error);
      })
      .finally(() => {
        setIsLoading(false);
      });
  }, [moveHistory, game, isPlayerTurn, isLoading, makeLLMMove]);

  return (
    <div className="app-container" style={{ width: '400px', margin: '0 auto' }}>
      <div className="llm-comment">
        <p>{isLoading ? <ClipLoader color="white" /> : llmComment}</p>
      </div>
      <Chessboard
        id="BasicBoard"
        boardWidth={400}
        position={game.fen()}
        onPieceDrop={onDrop}
        autoPromoteToQueen={true}
      />
      <div style={{ marginTop: '20px' }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '10px' }}>
          {moveHistory.map((move, index) => (
            <span key={index}>
              {index % 2 === 0 ? `${Math.floor(index / 2 + 1)}. ` : ''}{move}{' '}
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}
