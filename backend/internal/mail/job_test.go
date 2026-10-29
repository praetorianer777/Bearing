package mail

import (
	"context"
	"encoding/json"
	"testing"
)

type kept []Mail

func (k *kept) Send(_ context.Context, m Mail) error {
	*k = append(*k, m)
	return nil
}

func TestTheJobSendsTheMailItCarries(t *testing.T) {
	var sent kept
	payload, _ := json.Marshal(Mail{To: "a@b.test", Subject: "Hi", Body: "Hello"})
	if err := Handler(&sent)(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0].To != "a@b.test" || sent[0].Body != "Hello" {
		t.Errorf("sent %+v", sent)
	}
	if err := Handler(&sent)(context.Background(), json.RawMessage(`[`)); err == nil {
		t.Error("a broken job was sent")
	}
}
