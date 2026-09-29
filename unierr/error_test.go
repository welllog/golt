package unierr

import (
	"encoding/json"
	"testing"

	"github.com/welllog/golib/testz"
	"google.golang.org/grpc/codes"
)

func TestError_MarshalJSON_EscapeMsg(t *testing.T) {
	msg := `he said "hi" \ ok`

	b, err := New(1000, msg).MarshalJSON()
	testz.Nil(t, err)

	var got map[string]any
	testz.Nil(t, json.Unmarshal(b, &got))

	m, _ := got["msg"].(string)
	testz.Equal(t, msg, m)
}

func TestError_SetMethodsMutateReceiver(t *testing.T) {
	// documented contract: Set* mutates the receiver; a shared package-level
	// *Error must not be passed through them
	base := New(1000, "shared")

	variant := base.SetHttpCode(401).SetData("extra")
	if base.HttpCode() != 401 || variant.HttpCode() != 401 {
		t.Fatal("SetHttpCode should mutate the receiver")
	}
	if variant.Message() != "shared" || variant.Code() != 1000 {
		t.Fatal("chained Set should return the same error")
	}
}

func TestError_DefaultHttpCodeByGrpcCode(t *testing.T) {
	cases := []struct {
		code int
		want int
	}{
		{int(codes.InvalidArgument), 400},
		{int(codes.Unauthenticated), 401},
		{int(codes.PermissionDenied), 403},
		{int(codes.NotFound), 404},
		{int(codes.AlreadyExists), 409},
		{int(codes.ResourceExhausted), 429},
		{int(codes.DeadlineExceeded), 504},
		{int(codes.Unimplemented), 501},
		{int(codes.Unavailable), 503},
		{int(codes.Internal), 500},
		{int(codes.DataLoss), 500},
		{int(codes.Unknown), 400},
		{1000, 400}, // custom business codes keep the old default
	}

	for _, c := range cases {
		if got := New(c.code, "m").HttpCode(); got != c.want {
			t.Errorf("code %d: want http %d, got %d", c.code, c.want, got)
		}
	}

	// explicit SetHttpCode always wins
	if got := New(int(codes.Internal), "m").SetHttpCode(418).HttpCode(); got != 418 {
		t.Errorf("SetHttpCode overridden, got %d", got)
	}
}
