package app

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestChatAPIPagesImagesAndCleanup(t *testing.T) {
	var pngData bytes.Buffer
	png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "tenant_access_token"):
			w.Write([]byte(`{"code":0,"tenant_access_token":"test-only","expire":7200}`))
		case r.URL.Path == "/open-apis/bot/v3/info":
			w.Write([]byte(`{"code":0,"bot":{"open_id":"ou_bot"}}`))
		case strings.Contains(r.URL.Path, "/resources/"):
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngData.Bytes())
		case r.URL.Path == "/open-apis/im/v1/messages/om_quote":
			w.Write([]byte(`{"code":0,"data":{"items":[{"message_id":"om_quote","chat_id":"oc_chat","create_time":"500","msg_type":"image","body":{"content":"{\"image_key\":\"img_test\"}"}}]}}`))
		case r.URL.Path == "/open-apis/im/v1/messages":
			q := r.URL.Query()
			if q.Get("container_id") != "oc_chat" || q.Get("sort_type") != "ByCreateTimeDesc" || q.Get("end_time") != "2" {
				t.Errorf("bad history query %v", q)
			}
			if q.Get("page_token") == "next" {
				w.Write([]byte(`{"code":0,"data":{"items":[],"has_more":false}}`))
				return
			}
			w.Write([]byte(`{"code":0,"data":{"items":[{"message_id":"om_history","chat_id":"oc_chat","create_time":"900","msg_type":"text","body":{"content":"{\"text\":\"已有故障\"}"}},{"message_id":"om_foreign","chat_id":"oc_elsewhere","create_time":"800"},{"message_id":"om_future","chat_id":"oc_chat","create_time":"2000"}],"has_more":true,"page_token":"next"}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client := lark.NewClient("chat-test", "test-only", lark.WithOpenBaseUrl(server.URL))
	if id, e := botIdentity(context.Background(), client); e != nil || id != "ou_bot" {
		t.Fatalf("identity: %q %v", id, e)
	}
	m := &larkim.EventMessage{MessageId: ptr("om_trigger"), ChatId: ptr("oc_chat"), ChatType: ptr("group"), CreateTime: ptr("1000"), ParentId: ptr("om_quote"), MessageType: ptr("text"), Content: ptr(`{"text":"分析"}`)}
	prompt, images, cleanup := prepareChat(client)(context.Background(), m, "分析")
	defer cleanup()
	if len(images) != 1 || !strings.Contains(prompt, "已有故障") || strings.Contains(prompt, "om_foreign") || strings.Contains(prompt, "om_future") {
		t.Fatalf("images=%d prompt=%s", len(images), prompt)
	}
	dir := filepath.Dir(images[0])
	if s, e := os.Stat(dir); e != nil || s.Mode().Perm() != 0700 {
		t.Fatal("directory permissions")
	}
	if s, e := os.Stat(images[0]); e != nil || s.Mode().Perm() != 0600 {
		t.Fatal("image permissions")
	}
	c := &chatTask{client: client, chat: "oc_chat", cutoff: 1000, dir: dir, known: map[string]*larkim.Message{}, pages: map[string]bool{}}
	first, e := c.list(context.Background(), "", "", 10)
	if e != nil || len(first.Messages) != 1 {
		t.Fatalf("%+v %v", first, e)
	}
	if _, e = c.list(context.Background(), "forged", "", 20); e == nil {
		t.Fatal("accepted forged cursor")
	}
	if _, e = c.list(context.Background(), "next", "", 20); e != nil {
		t.Fatal(e)
	}
	if _, e = c.list(context.Background(), "", "omt_unknown", 20); e == nil {
		t.Fatal("accepted foreign thread")
	}
	cleanup()
	if _, e = os.Stat(dir); !os.IsNotExist(e) {
		t.Fatal("task files retained")
	}
}

func TestContextMissingContinues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "tenant_access_token") {
			w.Write([]byte(`{"code":0,"tenant_access_token":"test-only","expire":7200}`))
			return
		}
		w.Write([]byte(`{"code":999,"msg":"private detail"}`))
	}))
	defer server.Close()
	c := lark.NewClient("missing-test", "test-only", lark.WithOpenBaseUrl(server.URL))
	m := &larkim.EventMessage{MessageId: ptr("om_trigger"), ChatId: ptr("oc_chat"), ChatType: ptr("group"), CreateTime: ptr("1000"), ParentId: ptr("om_quote"), MessageType: ptr("text"), Content: ptr(`{"text":"检查"}`)}
	prompt, _, cleanup := prepareChat(c)(context.Background(), m, "检查")
	defer cleanup()
	if !strings.HasPrefix(prompt, "检查") || !strings.Contains(prompt, "缺失") || strings.Contains(prompt, "private detail") {
		t.Fatal(prompt)
	}
}

func TestQuotedBotCardContext(t *testing.T) {
	content := `{"title":"Hyperliquid HTTP限流","elements":[[{"tag":"text","text":"hyperliquid /info http 429: null"}]]}`
	card := &larkim.Message{MessageId: ptr("om_card"), ChatId: ptr("oc_chat"), CreateTime: ptr("500"), MsgType: ptr("interactive"), Body: &larkim.MessageBody{Content: &content}, Sender: &larkim.Sender{Id: ptr("bot_alert"), SenderType: ptr("app")}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "tenant_access_token"):
			w.Write([]byte(`{"code":0,"tenant_access_token":"test-only","expire":7200}`))
		case r.URL.Path == "/open-apis/im/v1/messages/om_card", r.URL.Path == "/open-apis/im/v1/messages":
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []*larkim.Message{card}, "has_more": false}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client := lark.NewClient("card-context-test", "test-only", lark.WithOpenBaseUrl(server.URL))
	m := &larkim.EventMessage{MessageId: ptr("om_trigger"), ChatId: ptr("oc_chat"), ChatType: ptr("group"), CreateTime: ptr("1000"), ParentId: ptr("om_card"), MessageType: ptr("text"), Content: ptr(`{"text":"排查下"}`)}
	prompt, images, cleanup := prepareChat(client)(context.Background(), m, "排查下")
	defer cleanup()
	if !strings.Contains(prompt, "直接引用消息") || !strings.Contains(prompt, "Hyperliquid HTTP限流") || strings.Count(prompt, "hyperliquid /info http 429: null") != 1 || strings.Contains(prompt, "未解析") || len(images) != 0 {
		t.Fatalf("images=%v prompt=%s", images, prompt)
	}
	c := &chatTask{client: client, chat: "oc_chat", cutoff: 1000, known: map[string]*larkim.Message{}, pages: map[string]bool{}}
	page, err := c.list(context.Background(), "", "", 10)
	if err != nil || len(page.Messages) != 1 || !strings.Contains(page.Messages[0].Text, "http 429") {
		t.Fatalf("history=%+v err=%v", page, err)
	}
}

func TestContextSocketRejectsOutsideMessage(t *testing.T) {
	c := &chatTask{known: map[string]*larkim.Message{}}
	r := httptest.NewRequest("POST", "/context", strings.NewReader(`{"action":"image","message":"om_other","index":0}`))
	w := httptest.NewRecorder()
	c.serve(w, r)
	var result contextResult
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Error == "" {
		t.Fatal(w.Body.String())
	}
}

func TestFeishuDownloadLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(bytes.Repeat([]byte("x"), 20*1024*1024+1)) }))
	defer server.Close()
	req, _ := http.NewRequest("GET", server.URL+"/resources/test", nil)
	if _, e := newFeishuHTTP().Do(req); e == nil {
		t.Fatal("unbounded resource response")
	}
}
