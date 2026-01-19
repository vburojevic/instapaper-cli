package instapaper

import (
	"encoding/json"
	"math"
	"testing"
)

func TestFolderPositionFloat(t *testing.T) {
	body := []byte(`{"type":"folder","folder_id":1,"title":"AI","position":1768855200.6905866}`)
	var f Folder
	if err := json.Unmarshal(body, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := 1768855200.6905866
	if diff := math.Abs(float64(f.Position) - want); diff > 1e-9 {
		t.Fatalf("position=%v want=%v diff=%v", f.Position, want, diff)
	}
}

func TestInt64UnmarshalFloat(t *testing.T) {
	var v Int64
	if err := json.Unmarshal([]byte(`1768856911.4784305`), &v); err != nil {
		t.Fatalf("unmarshal float: %v", err)
	}
	if int64(v) != 1768856911 {
		t.Fatalf("value=%d", int64(v))
	}

	if err := json.Unmarshal([]byte(`"1768856911.4784305"`), &v); err != nil {
		t.Fatalf("unmarshal float string: %v", err)
	}
	if int64(v) != 1768856911 {
		t.Fatalf("value=%d", int64(v))
	}
}
