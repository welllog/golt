package unierr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoimpl"
)

const (
	UnKnown  = int(codes.Unknown)
	Internal = int(codes.Internal)
)

var (
	_ error = (*Error)(nil)
)

type Error struct {
	err      error
	msg      string
	code     int
	httpCode int
	details  []proto.Message
	data     any
}

func New(code int, msg string) *Error {
	return Wrap(nil, code, msg)
}

func Newf(code int, format string, args ...any) *Error {
	return Wrap(nil, code, fmt.Sprintf(format, args...))
}

func Wrap(err error, code int, msg string) *Error {
	e := Error{
		err:      err,
		msg:      msg,
		code:     code,
		httpCode: http.StatusBadRequest,
	}

	return &e
}

func Wrapf(err error, code int, format string, args ...any) *Error {
	return Wrap(err, code, fmt.Sprintf(format, args...))
}

func FromStatus(s *status.Status) *Error {
	e := Wrap(nil, int(s.Code()), s.Message())
	st := s.Proto()
	for _, d := range st.Details {
		e.details = append(e.details, d)
	}
	return e
}

func FromStatusErr(err error) *Error {
	return FromStatus(status.Convert(err))
}

func (e *Error) WithHttpCode(httpCode int) *Error {
	e.httpCode = httpCode
	return e
}

func (e *Error) WithData(data any) *Error {
	e.data = data
	return e
}

func (e *Error) WithDetails(details ...proto.Message) *Error {
	e.details = append(e.details, details...)
	return e
}

func (e *Error) Error() string {
	msg := "[" + strconv.Itoa(e.code) + "]" + e.msg
	if e.err != nil {
		msg += "; raw: " + e.err.Error()
	}
	return msg
}

func (e *Error) GRPCStatus() *status.Status {
	st := status.New(codes.Code(e.code), e.msg)
	for _, d := range e.details {
		nst, err := st.WithDetails(protoimpl.X.ProtoMessageV1Of(d))
		if err == nil {
			st = nst
		}
	}
	return st
}

func (e *Error) Code() int {
	return e.code
}

func (e *Error) Message() string {
	return e.msg
}

func (e *Error) HttpCode() int {
	return e.httpCode
}

func (e *Error) Unwrap() error {
	return e.err
}

func (e *Error) LastDetail() proto.Message {
	if len(e.details) == 0 {
		return nil
	}
	return e.details[len(e.details)-1]
}

func (e *Error) Data() any {
	return e.data
}

func (e *Error) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(128)

	buf.WriteString(`{"code":`)
	buf.WriteString(strconv.Itoa(e.code))
	buf.WriteString(`,"msg":"`)
	buf.WriteString(e.msg)
	buf.WriteString(`"`)

	var (
		err error
		b   []byte
	)
	if e.data != nil {
		buf.WriteString(`,"data":`)
		err = json.NewEncoder(&buf).Encode(e.data)
		b = buf.Bytes()
		if b[len(b)-1] == '\n' {
			b = b[:len(b)-1]
		}
	} else if len(e.details) > 0 {
		buf.WriteString(`,"data":`)
		b = buf.Bytes()
		b, err = pbMarshaler.MarshalAppend(b, e.details[len(e.details)-1])
	} else {
		b = buf.Bytes()
	}

	if err != nil {
		return nil, err
	}

	b = append(b, '}')
	return b, nil
}

func (e *Error) UnmarshalJSON(data []byte) error {
	var jsonRepresentation errResponse
	if err := json.Unmarshal(data, &jsonRepresentation); err != nil {
		return err
	}

	e.code = jsonRepresentation.Code
	e.msg = jsonRepresentation.Message

	return nil
}

var pbMarshaler = protojson.MarshalOptions{
	AllowPartial:    true,
	UseProtoNames:   true,
	UseEnumNumbers:  true,
	EmitUnpopulated: true,
}

type errResponse struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
}
