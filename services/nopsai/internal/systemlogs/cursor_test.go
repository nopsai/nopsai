package systemlogs

import (
	"errors"
	"testing"
	"time"
)

func TestCursorCodecRoundTripAndRejectsTampering(t *testing.T) {
	codec := NewCursorCodec([]byte("test-key"))
	want := Cursor{SourceID: "dispatcher", ContainerInstance: "abc", Sequence: 42, EmittedAt: time.Unix(10, 20).UTC()}
	token, err := codec.Encode(want)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	got, err := codec.Decode(token)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if got.SourceID != want.SourceID || got.ContainerInstance != want.ContainerInstance || got.Sequence != want.Sequence || !got.EmittedAt.Equal(want.EmittedAt) {
		t.Fatalf("Decode() = %#v, want %#v", got, want)
	}
	tampered := token[:len(token)-1] + "A"
	if _, err := codec.Decode(tampered); !errors.Is(err, ErrCursorInvalid) {
		t.Fatalf("Decode(tampered) error = %v, want ErrCursorInvalid", err)
	}
}

func TestCursorCodecRejectsMalformedPayload(t *testing.T) {
	codec := NewCursorCodec([]byte("test-key"))
	for _, token := range []string{"", "not-base64", "YQ"} {
		if _, err := codec.Decode(token); !errors.Is(err, ErrCursorInvalid) {
			t.Fatalf("Decode(%q) error = %v, want ErrCursorInvalid", token, err)
		}
	}
}
