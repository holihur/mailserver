package blob

import (
	"bytes"
	"testing"
)

func TestPutGetDelete(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Put([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 64 {
		t.Fatalf("id 应为 sha256 hex，得到 %q", id)
	}
	b, err := s.Get(id)
	if err != nil || !bytes.Equal(b, []byte("hello")) {
		t.Fatalf("Get=%q err=%v", b, err)
	}
	// 相同内容去重：同一 id
	id2, _ := s.Put([]byte("hello"))
	if id2 != id {
		t.Fatalf("相同内容应去重，得到 %s vs %s", id, id2)
	}
	// 不同内容不同 id
	if id3, _ := s.Put([]byte("world")); id3 == id {
		t.Fatal("不同内容不应同 id")
	}
	if err := s.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(id); err == nil {
		t.Fatal("删除后应读不到")
	}
	// 再次删除不报错
	if err := s.Delete(id); err != nil {
		t.Fatalf("重复删除应无错: %v", err)
	}
}

func TestBadID(t *testing.T) {
	s, _ := New(t.TempDir())
	for _, bad := range []string{"", "../etc/passwd", "abc", "zzzz"} {
		if _, err := s.Get(bad); err == nil {
			t.Fatalf("非法 id %q 应报错", bad)
		}
		if err := s.Delete(bad); err == nil {
			t.Fatalf("非法 id %q 删除应报错", bad)
		}
	}
}
