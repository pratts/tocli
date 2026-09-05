package process

import "testing"

func TestBootID_StableAcrossCalls(t *testing.T) {
	first, err := BootID()
	if err != nil {
		t.Fatalf("BootID: %v", err)
	}
	if first == "" {
		t.Fatal("BootID returned an empty string")
	}

	second, err := BootID()
	if err != nil {
		t.Fatalf("BootID (second call): %v", err)
	}
	if first != second {
		t.Fatalf("BootID not stable within the same boot session: %q != %q", first, second)
	}
}
