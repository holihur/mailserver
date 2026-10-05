package handler

import "testing"

func TestSplitMessages(t *testing.T) {
	eml := "From: a@b.c\r\nTo: d@e.f\r\nSubject: hi\r\n\r\nbody\r\n"
	if got := splitMessages(eml); len(got) != 1 {
		t.Fatalf("单封 EML 应 1 条，得到 %d", len(got))
	}
	mbox := "From a@b.c Mon Jan  1 00:00:00 2024\r\nFrom: a@b.c\r\nSubject: m1\r\n\r\nbody1\r\n" +
		"From d@e.f Mon Jan  1 00:00:01 2024\r\nFrom: d@e.f\r\nSubject: m2\r\n\r\nbody2\r\n"
	got := splitMessages(mbox)
	if len(got) != 2 {
		t.Fatalf("mbox 应 2 条，得到 %d", len(got))
	}
	if from, _ := fromTo(got[1]); from != "d@e.f" {
		t.Fatalf("第二封 from=%q，得到 %v", from, got[1])
	}
}
