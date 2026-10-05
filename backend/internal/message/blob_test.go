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

// #6：配额按附件真实字节统计，而不是 blob 元数据/base64 长度。
func TestAttachmentRealSize(t *testing.T) {
	bs, err := blob.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	SetBlobStore(bs)
	defer SetBlobStore(nil)

	raw := []byte("0123456789") // 10 字节
	data := base64.StdEncoding.EncodeToString(raw)

	// 缺 size 的旧 base64 附件：按解码长度计算
	oldJSON := fmt.Sprintf(`[{"name":"a.bin","type":"application/octet-stream","data":%q}]`, data)
	if got := AttachmentsBytes(oldJSON); got != int64(len(raw)) {
		t.Fatalf("base64 附件应为 %d，得到 %d", len(raw), got)
	}

	// blobify 后应写入真实 size，且 AttachmentsBytes 不变
	blobbed := Blobify(oldJSON)
	atts := ParseAttachments(blobbed)
	if len(atts) != 1 || atts[0].Size != len(raw) {
		t.Fatalf("Blobify 应回填 Size=%d: %+v", len(raw), atts)
	}
	if got := AttachmentsBytes(blobbed); got != int64(len(raw)) {
		t.Fatalf("blob 附件应为 %d，得到 %d", len(raw), got)
	}

	// size=0 的 blob 附件也应能从磁盘还原真实大小
	blobbed = strings.Replace(blobbed, fmt.Sprintf(`"size":%d`, len(raw)), `"size":0`, 1)
	if got := AttachmentsBytes(blobbed); got != int64(len(raw)) {
		t.Fatalf("size=0 的 blob 附件应为 %d，得到 %d", len(raw), got)
	}
}
