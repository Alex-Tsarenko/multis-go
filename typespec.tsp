import "@typespec/http";
import "@typespec/rest";

using TypeSpec.Http;
using TypeSpec.Rest;

@service({
  title: "Multis.Agent API",
  version: "1.0.0",
})
@server("http://localhost:8080", "Локальный сервер")
namespace MultisAgent {
  #region Models

  enum SessionStatus {
    idle: "idle",
    running: "running",
    stopped: "stopped",
    error: "error",
  }

  enum FinishReason {
    stop: "stop",
    max_steps: "max_steps",
    loop_detected: "loop_detected",
    error: "error",
  }

  enum SecurityMode {
    default: "default",
    ask: "ask",
    auto: "auto",
  }

  model PromptHistoryItem {
    id: string;
    session_id?: string;
    prompt: string;
    created_at: utcDateTime;
  }

  model AppSettings {
    last_model?: string;
    security_mode?: SecurityMode;
  }

  model UpdateAppSettingsRequest {
    last_model?: string;
    security_mode?: SecurityMode;
  }

  model UnloadModelRequest {
    model: string;
  }

  model Session {
    id: string;
    model: string;
    max_steps: int32;
    workspace: string;
    security_mode?: SecurityMode;
    status: SessionStatus;
    created_at: utcDateTime;
  }

  model CreateSessionRequest {
    model: string;
    @minValue(1)
    max_steps?: int32 = 50;
    workspace: string;
    security_mode?: SecurityMode = SecurityMode.default;
  }

  model SendMessageRequest {
    content: string;
  }

  model ErrorResponse {
    error: string;
    message: string;
  }

  #region SSE Events

  enum SSEEventType {
    reasoning: "reasoning",
    message: "message",
    tool_call: "tool_call",
    tool_result: "tool_result",
    error: "error",
    done: "done",
  }

  model ReasoningData {
    content: string;
  }

  model MessageData {
    content: string;
  }

  model ToolCallData {
    id: string;
    name: string;
    arguments: Record<unknown>;
  }

  model ToolResultData {
    status: "success" | "error";
    output: string;
  }

  model ErrorData {
    message: string;
  }

  model DoneData {
    finish_reason: FinishReason;
  }

  model SSEEvent {
    type: SSEEventType;
    data: ReasoningData | MessageData | ToolCallData | ToolResultData | ErrorData | DoneData;
  }

  #endregion

  #region Operations

  @route("/session")
  @post
  op createSession(@body body: CreateSessionRequest): {
    @statusCode statusCode: 201;
    @body session: Session;
  } | {
    @statusCode statusCode: 400;
    @body error: ErrorResponse;
  };

  @route("/session/{id}")
  @get
  op getSession(@path id: string): {
    @statusCode statusCode: 200;
    @body session: Session;
  } | {
    @statusCode statusCode: 404;
    @body error: ErrorResponse;
  };

  @route("/session/{id}")
  @delete
  op deleteSession(@path id: string): {
    @statusCode statusCode: 204;
  } | {
    @statusCode statusCode: 404;
    @body error: ErrorResponse;
  };

  @route("/session/{id}/message")
  @post
  op sendMessage(@path id: string, @body body: SendMessageRequest): {
    @statusCode statusCode: 200;
    @body status: { status: "accepted" };
  } | {
    @statusCode statusCode: 400;
    @body error: ErrorResponse;
  } | {
    @statusCode statusCode: 404;
    @body error: ErrorResponse;
  };

  @route("/session/{id}/stream")
  @get
  op streamSession(@path id: string): {
    @statusCode statusCode: 200;
    @header("Content-Type") contentType: "text/event-stream";
    @body stream: string;
  } | {
    @statusCode statusCode: 404;
    @body error: ErrorResponse;
  };

  @route("/session/{id}/stop")
  @post
  op stopSession(@path id: string): {
    @statusCode statusCode: 200;
    @body status: { status: "stopped" };
  } | {
    @statusCode statusCode: 404;
    @body error: ErrorResponse;
  };

  @route("/prompts/history")
  @get
  op getPromptHistory(@query limit?: int32 = 50): {
    @statusCode statusCode: 200;
    @body history: PromptHistoryItem[];
  };

  @route("/prompts/history")
  @delete
  op clearPromptHistory(): {
    @statusCode statusCode: 204;
  };

  @route("/settings")
  @get
  op getSettings(): {
    @statusCode statusCode: 200;
    @body settings: AppSettings;
  };

  @route("/settings")
  @put
  op updateSettings(@body body: UpdateAppSettingsRequest): {
    @statusCode statusCode: 200;
    @body settings: AppSettings;
  } | {
    @statusCode statusCode: 400;
    @body error: ErrorResponse;
  };

  @route("/models/unload")
  @post
  op unloadModel(@body body: UnloadModelRequest): {
    @statusCode statusCode: 200;
    @body status: { status: "unloaded", model: string };
  } | {
    @statusCode statusCode: 400;
    @body error: ErrorResponse;
  } | {
    @statusCode statusCode: 500;
    @body error: ErrorResponse;
  };

  #endregion
}
