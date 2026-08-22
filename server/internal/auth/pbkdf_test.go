package auth

import (
	"encoding/hex"
	"testing"
)

func TestDeriveKeyMatchesPBKDF2SHA256Vector(t *testing.T) {
	want, err := hex.DecodeString("120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b")
	if err != nil {
		t.Fatal(err)
	}
	got := deriveKey([]byte("password"), []byte("salt"), 1, 32)
	if string(got) != string(want) {
		t.Fatalf("derived key = %x, want %x", got, want)
	}
}
