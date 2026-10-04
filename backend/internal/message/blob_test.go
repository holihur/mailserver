package message

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"mailserver/internal/blob"
)

// #6：附件 base64 -> blob 落盘 -> 读时恢复 base64。
func TestBlobifyHydrate(t *testing.T) {
	bs, err := blob.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	SetBlobStore(bs)
	defer SetBlobStore(nil)

	data := base64.StdEncoding.EncodeToString([]byte("hello-blob"))
	atts := fmt.Sprintf(`[{"name":"a.txt","type":"text/plain","data":%q,"size":10}]`, data)

	blobbed := Blobify(atts)
	if strings.Contains(blobbed, data) {
		t.Fatalf("blob 化后不应再含 base64: %s", blobbed)
	}
	if !strings.Contains(blobbed, `"blob"`) {
		t.Fatalf("应含 blob 字段: %s", blobbed)
	}
	hydrated := HydrateAttachments(blobbed)
	if !strings.Contains(hydrated, data) {
		t.Fatalf("应恢复 base64: %s", hydrated)
	}
	// Bytes() 也应能从 blob 读出
	parsed := ParseAttachments(blobbed)
	if len(parsed) != 1 || string(parsed[0].Bytes()) != "hello-blob" {
		t.Fatalf("Bytes() 读取失败: %+v", parsed)
	}
}
