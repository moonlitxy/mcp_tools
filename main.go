package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// JSONRPCRequest 表示 JSON-RPC 2.0 请求结构
// 为什么这样定义：MCP 基于 JSON-RPC 2.0，服务端需要按协议解析 method 与 params，以支持 initialize/tools 等方法
type JSONRPCRequest struct {
	Jsonrpc string          `json:"jsonrpc"`          // 固定为 "2.0"，确保兼容性
	ID      json.RawMessage `json:"id,omitempty"`     // 请求 ID 可以是 string 或 number，RawMessage 便于透传
	Method  string          `json:"method"`           // 方法名，如 initialize、tools/list、tools/call
	Params  json.RawMessage `json:"params,omitempty"` // 参数体，延迟解码以便不同方法使用不同结构
}

// JSONRPCResponse 表示 JSON-RPC 2.0 响应结构
// 为什么这样定义：返回结果或错误必须二选一，ID 必须与请求一致
type JSONRPCResponse struct {
	Jsonrpc string           `json:"jsonrpc"`
	ID      json.RawMessage  `json:"id"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *JSONRPCErrorObj `json:"error,omitempty"`
}

// JSONRPCErrorObj 表示 JSON-RPC 错误返回体
// 为什么这样定义：遵循 JSON-RPC 错误格式，包含 code/message，data 可选
type JSONRPCErrorObj struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// InitializeParams 表示客户端发来的 initialize 参数
// 为什么这样定义：生命周期握手需要版本与客户端能力，服务端只需读取版本即可回传自身能力
type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]any         `json:"capabilities,omitempty"`
	ClientInfo      map[string]interface{} `json:"clientInfo,omitempty"`
}

// InitializeResult 表示服务端返回的 initialize 结果
// 为什么这样定义：服务端须告知自身支持的能力，这里仅声明 tools 能力满足本工具场景
type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]any         `json:"capabilities"`
	ServerInfo      map[string]interface{} `json:"serverInfo"`
	Instructions    string                 `json:"instructions,omitempty"`
}

// ToolsListResult 表示 tools/list 返回体
// 为什么这样定义：根据 MCP Tools 规范，需要返回 tools 列表与可选分页游标
type ToolsListResult struct {
	Tools      []ToolDef `json:"tools"`
	NextCursor string    `json:"nextCursor,omitempty"`
}

// ToolDef 表示单个工具的定义
// 为什么这样定义：包含名称、描述及 JSON Schema 输入输出，便于宿主解析与函数调用映射
type ToolDef struct {
	Name         string          `json:"name"`
	Title        string          `json:"title,omitempty"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"inputSchema,omitempty"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
}

// ToolsCallParams 表示 tools/call 请求参数
// 为什么这样定义：MCP 约定 name + arguments 结构来调用具体工具
type ToolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolsCallResult 表示 tools/call 响应体
// 为什么这样定义：返回 text 内容与可选结构化内容，既满足向后兼容也利于调用方消费
type ToolsCallResult struct {
	Content           []ContentItem  `json:"content,omitempty"`
	StructuredContent map[string]any `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
}

// ContentItem 表示非结构化内容块（文本）
// 为什么这样定义：MCP 约定 content 可以包含多种类型，这里仅用 text 满足计算结果展示
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// TwoSumArgs 表示两数之和工具的参数
// 为什么这样定义：与输入 JSON Schema 保持一致，确保参数校验与业务处理分离
type TwoSumArgs struct {
	Nums   []int `json:"nums"`
	Target int   `json:"target"`
}

// twoSum 计算两数之和的索引（返回首个匹配）
// 为什么这样实现：使用哈希表 O(n) 时间复杂度，满足大规模数据性能需求
func twoSum(nums []int, target int) (int, int, bool) {
	m := make(map[int]int, len(nums)) // 值 -> 索引
	for i, v := range nums {
		if j, ok := m[target-v]; ok {
			return j, i, true
		}
		m[v] = i
	}
	return -1, -1, false
}

// writeResponse 将响应写入 stdout 并换行分帧
// 为什么这样做：stdio 传输通常按行分帧，便于客户端逐条解析
func writeResponse(w io.Writer, id json.RawMessage, result any, errObj *JSONRPCErrorObj) error {
	resp := JSONRPCResponse{Jsonrpc: "2.0", ID: id}
	if errObj != nil {
		resp.Error = errObj
	} else {
		b, e := json.Marshal(result)
		if e != nil {
			// 编码失败时返回通用错误，避免连接挂起
			resp.Error = &JSONRPCErrorObj{Code: -32603, Message: "Internal error: marshal result failed"}
		} else {
			resp.Result = b
		}
	}
	enc := json.NewEncoder(w)
	if err := enc.Encode(resp); err != nil {
		return err
	}
	return nil
}

// buildTwoSumSchemas 构造输入输出 JSON Schema
// 为什么这样做：服务端以原始 JSON 返回 Schema，避免引入第三方库并提升兼容性
func buildTwoSumSchemas() (json.RawMessage, json.RawMessage) {
	input := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"nums": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "integer"},
				"description": "整数数组",
			},
			"target": map[string]any{
				"type":        "integer",
				"description": "目标和",
			},
		},
		"required": []string{"nums", "target"},
	}
	output := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"indices": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "integer"},
				"minItems":    2,
				"maxItems":    2,
				"description": "满足两数之和的两个索引",
			},
		},
		"required": []string{"indices"},
	}
	ib, _ := json.Marshal(input)
	ob, _ := json.Marshal(output)
	return ib, ob
}

// handleInitialize 处理初始化握手
// 为什么这样做：必须先完成版本与能力协商，客户端才会继续发送工具请求
func handleInitialize(id json.RawMessage, params json.RawMessage, w io.Writer) error {
	var p InitializeParams
	_ = json.Unmarshal(params, &p) // 容错处理：若解析失败，使用默认版本
	// 按最新规范返回当前版本，若客户端版本不兼容由客户端自行断开
	result := InitializeResult{
		ProtocolVersion: "2025-03-26",
		Capabilities: map[string]any{
			"tools": map[string]any{"listChanged": true},
		},
		ServerInfo: map[string]any{
			"name":    "two-sum-mcp",
			"version": "0.1.0",
		},
		Instructions: "该服务提供两数之和工具：输入整数数组与目标值，返回满足目标和的两个索引",
	}
	return writeResponse(w, id, result, nil)
}

// handleToolsList 返回工具列表
// 为什么这样做：客户端通过 tools/list 发现可调用工具
func handleToolsList(id json.RawMessage, w io.Writer) error {
	in, out := buildTwoSumSchemas()
	tools := []ToolDef{
		{
			Name:         "two_sum",
			Title:        "Two Sum",
			Description:  "返回数组中两元素索引，使其和等于目标值",
			InputSchema:  in,
			OutputSchema: out,
		},
	}
	result := ToolsListResult{Tools: tools}
	return writeResponse(w, id, result, nil)
}

// handleToolsCall 执行工具调用
// 为什么这样做：将协议层参数解析与业务逻辑解耦，统一返回结构化与文本结果
func handleToolsCall(id json.RawMessage, params json.RawMessage, w io.Writer) error {
	var p ToolsCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return writeResponse(w, id, nil, &JSONRPCErrorObj{Code: -32602, Message: "Invalid params"})
	}
	if p.Name != "two_sum" {
		return writeResponse(w, id, nil, &JSONRPCErrorObj{Code: -32601, Message: "Method not found: unknown tool"})
	}

	var args TwoSumArgs
	if err := json.Unmarshal(p.Arguments, &args); err != nil {
		return writeResponse(w, id, nil, &JSONRPCErrorObj{Code: -32602, Message: "Invalid arguments for two_sum"})
	}

	i, j, ok := twoSum(args.Nums, args.Target)
	if !ok {
		res := ToolsCallResult{
			Content: []ContentItem{{Type: "text", Text: "未找到符合条件的两个索引"}},
			IsError: true,
		}
		return writeResponse(w, id, res, nil)
	}

	txt := fmt.Sprintf("indices: [%d,%d]", i, j)
	res := ToolsCallResult{
		Content:           []ContentItem{{Type: "text", Text: txt}},
		StructuredContent: map[string]any{"indices": []int{i, j}},
		IsError:           false,
	}
	return writeResponse(w, id, res, nil)
}

func main() {
	// 为什么使用 Scanner：简单稳定地逐条读取 JSON-RPC 消息；并提升缓冲以容纳较大消息体
	scanner := bufio.NewScanner(os.Stdin)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024) // 最大 10MB

	for scanner.Scan() {
		line := scanner.Bytes()
		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			// 输入非 JSON 时忽略，保持进程存活以便客户端恢复
			continue
		}

		switch req.Method {
		case "initialize":
			_ = handleInitialize(req.ID, req.Params, os.Stdout)
		case "tools/list":
			_ = handleToolsList(req.ID, os.Stdout)
		case "tools/call":
			_ = handleToolsCall(req.ID, req.Params, os.Stdout)
		default:
			// 未知方法返回标准错误，避免客户端阻塞
			_ = writeResponse(os.Stdout, req.ID, nil, &JSONRPCErrorObj{Code: -32601, Message: "Method not found"})
		}
	}
}
