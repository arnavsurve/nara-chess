import { useEffect, useState, useCallback, useRef } from "react";
import { Chessboard } from "react-chessboard";
import { Chess, Move, Square } from "chess.js";
import Loader from "./components/atoms/Loader.tsx"
import "./App.css";

/*
 * TODO
 * move history next to chat, clicking a move rolls back game state and jumps to that chat message
 * - continue conversation from there?
 * - nested game states? depending on how many tangents / lines you go on with chat
 */

interface ChatMessage {
  content: string;
  role: 'user' | 'model';
}

enum Side {
  White = "white",
  Black = "black",
}

export default function App() {
  const [game, setGame] = useState(new Chess());
  const [moveHistory, setMoveHistory] = useState<string[]>([]);
  const [isPlayerTurn, setIsPlayerTurn] = useState<boolean>(true);
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [hasError, setHasError] = useState<boolean>(false);
  const [chatMessages, setChatMessages] = useState<ChatMessage[]>([]);
  const [inputValue, setInputValue] = useState<string>("");
  const [llmArrows, setLLMArrows] = useState<Array<Array<string>>>([]);
  const [title, setTitle] = useState<string>("");
  const [userSide, setUserSide] = useState<Side>(Side.White);
  const MAX_RETRIES = 3;
  const chatContainerRef = useRef<HTMLDivElement>(null);

  const makePlayerMove = useCallback((move: Move): boolean => {
    if (!isPlayerTurn || isLoading) return false;

    const gameCopy = new Chess(game.fen());
    try {
      const result = gameCopy.move(move);
      if (result) {
        setGame(gameCopy);
        setMoveHistory(prev => [...prev, result.san]);
        setIsPlayerTurn(false);
        return true;
      }
    } catch (error) {
      console.warn("Invalid move attempt:", error);
      return false;
    } finally {
      setLLMArrows([]);
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
        setIsPlayerTurn(true);
        return false;
      }
    } catch (error) {
      console.error(`Error executing LLM move '${llmMoveSan}':`, error);
      setIsPlayerTurn(true);
      return false;
    }
  }, [game]);

  // Handle piece movement
  function onDrop(sourceSquare: Square, targetSquare: Square) {
    const move = makePlayerMove({
      from: sourceSquare,
      to: targetSquare,
      promotion: 'q' // default to queen promotion
    } as Move);

    // illegal move
    if (!move) return false;
    return true;
  }

  // Handle chat message
  async function sendMessage(newMessage: ChatMessage) {
    const payload = {
      message_history: [...chatMessages, newMessage].slice(-10),
      game_state: {
        move_history: moveHistory,
        fen: game.fen()
      },
      player_side: userSide
    }
    setIsLoading(true);
    try {
      const response = await fetch("http://localhost:42069/chat", {
        method: "POST",
        headers: {
          "Content-Type": "application/json"
        },
        body: JSON.stringify(payload),
      })

      if (!response.ok) {
        throw new Error("Failed to send data to endpoint");
      }

      const data = await response.json()
      setChatMessages(prev => [...prev, {
        content: data.response,
        role: 'model'
      }]);
      if (data.arrows.length) {
        setLLMArrows(data.arrows);
      }

    } catch (error) {
      console.error(error);
    } finally {
      setIsLoading(false);
    }
  }

  useEffect(() => {
    // Only proceed if it's not the player's turn and we're not already loading
    if (isPlayerTurn || isLoading || moveHistory.length < 1 || hasError) {
      return;
    }

    const makeRequest = async (attempt = 0) => {
      if (attempt >= MAX_RETRIES) {
        setHasError(true);
        setIsLoading(false);
        return;
      }

      setIsLoading(true);

      const payload = {
        move_history: moveHistory,
        fen: game.fen(),
        chat_history: chatMessages.slice(-5)
      }
      console.log("sending to api attempt:", attempt, payload);

      try {
        const response = await fetch("http://localhost:42069/generateMove", {
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
        setChatMessages(prev => [...prev, {
          content: data.comment,
          role: 'model'
        }]);
        const moveSuccess = makeLLMMove(data.move);
        setLLMArrows(data.arrows);
        setTitle(data.title);

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

  useEffect(() => {
    if (chatContainerRef.current) {
      chatContainerRef.current.scrollTop = chatContainerRef.current.scrollHeight;
    }
  }, [chatMessages, isLoading]);

  const handleRetry = () => {
    setHasError(false);
    setIsPlayerTurn(false);
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (inputValue.trim()) {
      const newMessage = {
        content: inputValue.trim(),
        role: 'user' as const
      };
      setChatMessages(prev => [...prev, newMessage]);
      setInputValue("");
      sendMessage(newMessage);
    }
  };

  return (
    <div className="app-container" style={{
      display: 'flex',
      flexDirection: 'column',
    }}>
      <div style={{
        display: 'flex',
        justifyContent: 'center'
      }}>
        <div className="left-column" style={{ padding: "16px" }}>
          <h3>{title}</h3>
          <div className="game-section">
            <Chessboard
              id="BasicBoard"
              boardWidth={400}
              position={game.fen()}
              onPieceDrop={onDrop}
              autoPromoteToQueen={true}
              customArrowColor="#2b5278"
              customArrows={llmArrows}
            />
          </div>
        </div>

        <div className="center-column" style={{
          padding: '16px',
          width: "96px",
          display: "flex",
          flexDirection: "column",
        }}>
          <div className="move-history" style={{
            display: 'flex',
            flexWrap: 'wrap',
            justifyContent: 'left',
            height: "80%",
            gap: "4px",
            alignContent: "flex-start"
          }}>
            {Array.from({ length: Math.ceil(moveHistory.length / 2) }).map((_, index) => (
              <span key={index} style={{ lineHeight: "1.2" }}>
                {`${index + 1}. ${moveHistory[index * 2]} ${moveHistory[index * 2 + 1] || ''}`}
              </span>
            ))}
          </div>
        </div>

        <div className="right-column">
          <div className="chat-window" style={{
            width: '300px',
            height: '640px',
            border: '1px solid #666',
            borderRadius: '8px',
            padding: '16px',
            display: 'flex',
            flexDirection: 'column',
            gap: '10px'
          }}>
            <div
              ref={chatContainerRef}
              style={{
                flexGrow: 1,
                overflowY: 'auto',
                display: 'flex',
                flexDirection: 'column'
              }}
            >
              <div style={{ height: '100%' }}>
                {chatMessages.map((msg, index) => (
                  <p key={index} style={{
                    margin: '8px 0',
                    borderRadius: '4px',
                    color: index === chatMessages.length - 1 ? 'inherit' : '#aaa',
                    textAlign: msg.role === 'user' ? 'right' : 'left',
                    backgroundColor: msg.role === 'user' ? '#2b5278' : '#383838',
                    padding: '8px 12px',
                  }}>
                    {msg.content}
                  </p>
                ))}
              </div>
            </div>
            <div style={{ justifyContent: "center", alignItems: "center", display: "flex" }}>
              {isLoading ? <Loader /> : null}
            </div>

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
                }}
              >
                Retry Move
              </button>
            )}

            <form onSubmit={handleSubmit} style={{
              display: 'flex',
              gap: '8px'
            }}>
              <input
                type="text"
                value={inputValue}
                onChange={(e) => setInputValue(e.target.value)}
                style={{
                  flex: 1,
                  padding: '8px',
                  borderRadius: '4px',
                  border: '1px solid #666',
                  backgroundColor: '#333',
                  color: 'white'
                }}
                placeholder="Ask anything..."
              />
              <button
                type="submit"
                style={{
                  padding: '8px',
                  borderRadius: '4px',
                  backgroundColor: '#4a4a4a',
                  color: 'white',
                  border: 'none',
                  cursor: 'pointer'
                }}
              >
                Send
              </button>
            </form>

          </div>

        </div>
      </div>

    </div>
  );
}



