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
		httpCode: grpcHTTPCode(code),
	}

	return &e
}

// grpcHTTPCode derives the default HTTP status from a gRPC code, aligned
// with the grpc-gateway mapping; custom business codes fall back to 400.
// SetHttpCode overrides it.
func grpcHTTPCode(code int) int {
	switch codes.Code(code) {
	case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange:
		return http.StatusBadRequest
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists, codes.Aborted:
		return http.StatusConflict
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.Canceled:
		return http.StatusRequestTimeout
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.Internal, codes.DataLoss:
		return http.StatusInternalServerError
	default: // OK or custom business codes
		return http.StatusBadRequest
	}
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

// SetHttpCode sets the HTTP status code, mutates the receiver and returns
// it for chaining. Do not call this on a shared package-level *Error:
// it would be visible to every later use of that error. Build a fresh
// New(...) instead when a variant is needed.
func (e *Error) SetHttpCode(httpCode int) *Error {
	e.httpCode = httpCode
	return e
}

// SetData sets the response data, mutates the receiver and returns it for
// chaining. Do not call this on a shared package-level *Error.
func (e *Error) SetData(data any) *Error {
	e.data = data
	return e
}

// SetDetails appends the details, mutates the receiver and returns it for
// chaining. Do not call this on a shared package-level *Error.
func (e *Error) SetDetails(details ...proto.Message) *Error {
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
	buf.WriteString(`,"msg":`)
	msgBytes, err := json.Marshal(e.msg)
	if err != nil {
		return nil, err
	}
	buf.Write(msgBytes)

	var b []byte
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
