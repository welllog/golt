package config

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/welllog/golib/testz"
)

type addr struct {
	Province string
	City     string
	Street   string
}

type work struct {
	Title  string
	Salary int
}

func TestConfigure_String(t *testing.T) {
	engine, err := FromFile("./config.yaml", nil)
	testz.Nil(t, err)
	defer engine.Close()

	name1, err := engine.String("test/demo1", "name")
	testz.Nil(t, err)

	name2, err := engine.String("test/demo2", "name")
	testz.Nil(t, err)

	testz.Equal(t, "demo1", name1)
	testz.Equal(t, name1, name2)
}

func TestConfigure_String_Empty(t *testing.T) {
	engine, err := FromFile("./config.yaml", nil)
	testz.Nil(t, err)
	defer engine.Close()

	s1, err := engine.String("test/demo1", "test_title")
	testz.Nil(t, err)
	testz.Equal(t, "", s1)

	s2, err := engine.String("test/demo1", "no_value")
	testz.Nil(t, err)
	testz.Equal(t, "", s2)

	s3, err := engine.String("test/demo3", "no_value")
	testz.Nil(t, err)
	testz.Equal(t, "", s3)

	s4, err := engine.String("test/demo2", "test_name")
	testz.Nil(t, err)
	testz.Equal(t, "", s4)

	s5, err := engine.String("test/demo3", "test_name")
	testz.Nil(t, err)
	testz.Equal(t, "", s5)
}

func TestConfigure_Int64(t *testing.T) {
	engine, err := FromFile("./config.yaml", nil)
	testz.Nil(t, err)
	defer engine.Close()

	no, err := engine.Int64("test/demo1", "no")
	testz.Nil(t, err)
	testz.Equal(t, int64(2), no)

	maxValue, err := engine.Int64("test/demo3", "max_value")
	testz.Nil(t, err)
	testz.Equal(t, int64(120), maxValue)
}

func TestConfigure_Float64(t *testing.T) {
	engine, err := FromFile("./config.yaml", nil)
	testz.Nil(t, err)
	defer engine.Close()

	testValue, err := engine.Float64("test/demo1", "test_value")
	testz.Nil(t, err)
	testz.Equal(t, 12.3, testValue)

	testValue, err = engine.Float64("test/demo3", "test_value")
	testz.Nil(t, err)
	testz.Equal(t, 12.3, testValue)
}

func TestConfigure_Bool(t *testing.T) {
	engine, err := FromFile("./config.yaml", nil)
	testz.Nil(t, err)
	defer engine.Close()

	b1, err := engine.Bool("test/demo1", "leader")
	testz.Nil(t, err)
	testz.Equal(t, true, b1)

	b2, err := engine.Bool("test/demo2", "leader")
	testz.Nil(t, err)
	testz.Equal(t, b1, b2)
}

func TestConfigure_YamlDecode(t *testing.T) {
	engine, err := FromFile("./config.yaml", nil)
	testz.Nil(t, err)
	defer engine.Close()

	var a addr
	err = engine.YamlDecode("test/demo1", "addr", &a)
	testz.Nil(t, err)

	testz.Equal(t, "sichuan", a.Province)
	testz.Equal(t, "chengdu", a.City)
	testz.Equal(t, "nanjing", a.Street)
}

func TestConfigure_JsonDecode(t *testing.T) {
	engine, err := FromFile("./config.yaml", nil)
	testz.Nil(t, err)
	defer engine.Close()

	var w work
	err = engine.JsonDecode("test/demo1", "work", &w)
	testz.Nil(t, err)

	testz.Equal(t, "engineer", w.Title)
	testz.Equal(t, 10000, w.Salary)
}

func TestConfigureDynamic(t *testing.T) {
	f, err := os.OpenFile("test1.yaml", os.O_RDWR, 0666)
	testz.Nil(t, err)
	defer f.Close()

	b, err := io.ReadAll(f)
	testz.Nil(t, err)

	engine, err := FromFile("./config.yaml", nil)
	testz.Nil(t, err)
	defer engine.Close()

	var num int32
	engine.OnKeyChange("test/demo1", "name", func(b []byte) error {
		num++
		return nil
	})

	name, err := engine.String("test/demo1", "name")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", name)

	_, _ = f.Seek(io.SeekStart, 0)
	_, _ = f.Write(b)
	time.Sleep(600 * time.Millisecond)
	name, err = engine.String("test/demo1", "name")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", name)

	b2 := bytes.Replace(b, []byte("demo1"), []byte("demo2"), 1)
	_, err = f.Seek(io.SeekStart, 0)
	testz.Nil(t, err)
	_, err = f.Write(b2)
	testz.Nil(t, err)
	time.Sleep(600 * time.Millisecond)
	name, err = engine.String("test/demo1", "name")
	testz.Nil(t, err)
	testz.Equal(t, "demo2", name)

	_, _ = f.Seek(io.SeekStart, 0)
	_, _ = f.Write(b)
	time.Sleep(600 * time.Millisecond)
	name, err = engine.String("test/demo1", "name")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", name)

	testz.Equal(t, int32(2), num, "change event should be triggered twice")
}
