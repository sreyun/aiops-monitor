package sqltoolkit

import "testing"

func TestHasDigestPlaceholdersQuoted(t *testing.T) {
	in := `select tel,name from user where tel='?'`
	if !HasDigestPlaceholders(in) {
		t.Fatal("expected digest '?' detection")
	}
	if HasDigestPlaceholders(`select tel,name from user where tel='17301655949'`) {
		t.Fatal("real literal must not look like digest")
	}
	if !HasDigestPlaceholders(`SELECT * FROM t WHERE id = ?`) {
		t.Fatal("unbound ?")
	}
	if !HasDigestPlaceholders(`SELECT * FROM t WHERE a = '?' AND b = 1`) {
		t.Fatal("quoted digest placeholder")
	}
}

func TestSubstituteDigestQuotedPlaceholders(t *testing.T) {
	in := `select tel from user where tel='?' and name="?"`
	out, ok := SubstituteDigestQuotedPlaceholders(in)
	if !ok {
		t.Fatal("expected change")
	}
	if HasDigestPlaceholders(out) {
		t.Fatalf("still has digest ph: %s", out)
	}
	if out != `select tel from user where tel=NULL and name=NULL` {
		t.Fatalf("got %s", out)
	}
}
