package main

import "testing"

func TestSanitizeAttachments(t *testing.T) {
	in := []Attachment{
		{Kind: "image", Name: "a.png", Data: "abc"},
		{Kind: "file", Name: "b.txt", Text: "hello"},
		{Kind: "file", Name: "empty"}, // dropped
		{Kind: "image", Data: ""},     // dropped
	}
	out := sanitizeAttachments(in)
	if len(out) != 2 {
		t.Fatalf("want 2, got %d", len(out))
	}
	if out[0].Kind != "image" || out[0].Text != "" {
		t.Fatalf("image sanitize failed: %+v", out[0])
	}
	if out[1].Kind != "file" || out[1].Data != "" || out[1].Text != "hello" {
		t.Fatalf("file sanitize failed: %+v", out[1])
	}
}

func TestTicketCommentAllowsAttachmentOnly(t *testing.T) {
	m := newTicketManager()
	tk, err := m.Create(Ticket{Title: "t1"}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Comment(tk.ID, "bob", "", []Attachment{{Kind: "image", Name: "x.png", Data: "qq"}})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := m.Get(tk.ID)
	if !ok || len(got.Comments) == 0 {
		t.Fatal("expected comment")
	}
	last := got.Comments[len(got.Comments)-1]
	if len(last.Attachments) != 1 {
		t.Fatalf("expected attachment on comment: %+v", last)
	}
}
