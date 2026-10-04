package message

import "testing"

// #15：Received 跳数统计与 Delivered-To 环路去重。
func TestReceivedCountAndDeliveredTo(t *testing.T) {
	raw := "Received: from a\r\nReceived: from b\r\nDelivered-To: x@y.z\r\nFrom: a@b\r\n\r\nbody\r\n"
	if n := ReceivedCount(raw); n != 2 {
		t.Fatalf("Received 应为 2，得到 %d", n)
	}
	if !HasDeliveredTo(raw, "x@y.z") {
		t.Fatal("应命中 Delivered-To")
	}
	if HasDeliveredTo(raw, "other@y.z") {
		t.Fatal("不应命中其它地址")
	}
	if ReceivedCount("From: a@b\r\n\r\nx") != 0 {
		t.Fatal("无 Received 应为 0")
	}
}
