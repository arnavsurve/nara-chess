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
  const [retryCount, setRetryCount] = useState<number>(0);
  const [hasError, setHasError] = useState<boolean>(false);
  const MAX_RETRIES = 3;

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
    if (isPlayerTurn || isLoading || moveHistory.length < 1 || hasError) {
      return;
    }

    const makeRequest = async (attempt = 0) => {
      if (attempt >= MAX_RETRIES) {
        setHasError(true);
        setLLMComment("The AI seems confused. Click retry to try again.");
        setIsLoading(false);
        return;
      }

      setIsLoading(true);

      const payload = {
        move_history: moveHistory,
        fen: game.fen()
      }
      console.log("sending to api attempt:", attempt, payload);

      try {
        const response = await fetch("http://localhost:42069/submitMove", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify(payload),
        });

        if (!response.ok) {
          throw new Error("Failed to send data to endpoint");
        }

        const data = await response.json();
        console.log("Response from server:", data);
        setLLMComment(data.comment);
        const moveSuccess = makeLLMMove(data.move);

        if (!moveSuccess) {
          // Add delay before next retry
          await new Promise(resolve => setTimeout(resolve, 1000));
          makeRequest(attempt + 1);
        } else {
          setIsLoading(false);
        }
      } catch (error) {
        console.error("Error:", error);
        // Add delay before next retry
        await new Promise(resolve => setTimeout(resolve, 1000));
        makeRequest(attempt + 1);
      }
    };

    makeRequest(0);
  }, [moveHistory, game, isPlayerTurn, isLoading, makeLLMMove, hasError]);

  // Modify the retry handler to be simpler
  const handleRetry = () => {
    setHasError(false);
    setIsPlayerTurn(false);
  };

  return (
    <div className="app-container" style={{
      width: '800px',
      margin: '0 auto',
      display: 'flex',
      flexDirection: 'column',
      gap: '20px'
    }}>
      <div style={{
        display: 'flex',
        gap: '20px',
        justifyContent: 'center'
      }}>
        <div className="game-section">
          <Chessboard
            id="BasicBoard"
            boardWidth={400}
            position={game.fen()}
            onPieceDrop={onDrop}
            autoPromoteToQueen={true}
          />
        </div>

        <div className="llm-comment" style={{
          width: '300px',
          border: '1px solid #666',
          borderRadius: '8px',
          padding: '16px',
          overflowY: 'auto',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          gap: '10px'
        }}>
          <p style={{ margin: 0 }}>
            {isLoading ? <ClipLoader color="white" /> : llmComment}
          </p>
          {hasError && !isLoading && (
            <button
              onClick={handleRetry}
              style={{
                padding: '8px 16px',
                borderRadius: '4px',
                backgroundColor: '#4a4a4a',
                color: 'white',
                border: 'none',
                cursor: 'pointer',
                marginTop: '10px'
              }}
            >
              Retry Move
            </button>
          )}
        </div>
      </div>

      <div style={{
        marginTop: '20px',
        textAlign: 'center'
      }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '10px', justifyContent: 'center' }}>
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



