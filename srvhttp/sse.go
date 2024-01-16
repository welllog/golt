package srvhttp

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type Event struct {
	Event string
	Id    string
	Retry uint
	Data  any
}

var fieldReplacer = strings.NewReplacer(
	"\n", "\\n",
	"\r", "\\r")

var dataReplacer = strings.NewReplacer(
	"\n", "\ndata:",
	"\r", "\\r")

var (
	idPrefix    = []byte("id:")
	eventPrefix = []byte("event:")
	retryPrefix = []byte("retry:")
	dataPrefix  = []byte("data:")
	lineFeed    = []byte{'\n'}

	headerKey = "inside_sse_header_add"
)

func (e Event) Encode(c *Context) error {
	if !c.GetBool(headerKey) {
		h := c.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		c.Set(headerKey, true)
	}

	buf := c.Buffer()
	buf.Reset()
	buf.Grow(64)

	if len(e.Id) > 0 {
		buf.Write(idPrefix)
		_, _ = fieldReplacer.WriteString(buf, e.Id)
		buf.Write(lineFeed)
	}

	if len(e.Event) > 0 {
		buf.Write(eventPrefix)
		_, _ = fieldReplacer.WriteString(buf, e.Event)
		buf.Write(lineFeed)
	}

	if e.Retry > 0 {
		buf.Write(retryPrefix)
		buf.WriteString(strconv.FormatUint(uint64(e.Retry), 10))
		buf.Write(lineFeed)
	}

	buf.Write(dataPrefix)
	switch kindOfData(e.Data) {
	case reflect.Struct, reflect.Slice, reflect.Map:
		err := json.NewEncoder(buf).Encode(e.Data)
		if err != nil {
			return err
		}
		buf.Write(lineFeed)
	default:
		_, _ = dataReplacer.WriteString(buf, fmt.Sprint(e.Data))
		buf.WriteString("\n\n")
	}

	_, err := c.Write(buf.Bytes())
	if err != nil {
		return err
	}

	c.Flush()

	return nil
}

func kindOfData(data interface{}) reflect.Kind {
	value := reflect.ValueOf(data)
	valueType := value.Kind()
	if valueType == reflect.Ptr {
		valueType = value.Elem().Kind()
	}
	return valueType
}
