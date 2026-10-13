package main

// encounters_filter_test.go — B29.1.2: тесты parseEncounterFilter и cursor.

import (
	"net/url"
	"testing"
)

func TestParseEncounterFilter_Defaults(t *testing.T) {
	f, err := parseEncounterFilter(url.Values{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if f.Limit != encounterListLimitDefault {
		t.Errorf("limit default = %d, want %d", f.Limit, encounterListLimitDefault)
	}
	if f.Offset != 0 {
		t.Errorf("offset default = %d, want 0", f.Offset)
	}
	if f.NoTotal {
		t.Errorf("no_total default = true, want false")
	}
	if f.From != nil || f.To != nil {
		t.Errorf("from/to default should be nil")
	}
	if f.Order != encounterOrderDesc {
		t.Errorf("order default = %q, want %q", f.Order, encounterOrderDesc)
	}
}

func TestParseEncounterFilter_OK(t *testing.T) {
	q := url.Values{}
	q.Set("patient_id", "PAT-1")
	q.Set("kind", "consultation")
	q.Set("status", "finished")
	q.Set("specialty", "cardio")
	q.Set("from", "2026-01-01T00:00:00Z")
	q.Set("to", "2026-06-01T00:00:00Z")
	q.Set("limit", "50")
	q.Set("offset", "10")
	q.Set("no_total", "1")
	q.Set("order", "asc")

	f, err := parseEncounterFilter(q)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if f.PatientID != "PAT-1" || f.Kind != "consultation" || f.Status != "finished" ||
		f.Specialty != "cardio" || f.Limit != 50 || f.Offset != 10 || !f.NoTotal {
		t.Errorf("bad parse: %+v", f)
	}
	if f.Order != encounterOrderAsc {
		t.Errorf("order = %q, want %q", f.Order, encounterOrderAsc)
	}
	if f.From == nil || f.To == nil {
		t.Fatalf("from/to nil")
	}
	if !f.From.Before(*f.To) {
		t.Errorf("from should be < to")
	}
}

func TestParseEncounterFilter_Errors(t *testing.T) {
	cases := []struct {
		name string
		q    url.Values
	}{
		{"bad kind", url.Values{"kind": {"xyz"}}},
		{"bad status", url.Values{"status": {"open"}}},
		{"limit 0", url.Values{"limit": {"0"}}},
		{"limit 201", url.Values{"limit": {"201"}}},
		{"limit nan", url.Values{"limit": {"abc"}}},
		{"offset -1", url.Values{"offset": {"-1"}}},
		{"offset 10001", url.Values{"offset": {"10001"}}},
		{"from bad", url.Values{"from": {"not-iso"}}},
		{"to bad", url.Values{"to": {"2026/01/01"}}},
		{"from > to", url.Values{
			"from": {"2026-06-01T00:00:00Z"},
			"to":   {"2026-01-01T00:00:00Z"},
		}},
		{"bad order", url.Values{"order": {"xyz"}}},
		{"asc + cursor", url.Values{
			"order":  {"asc"},
			"cursor": {"bm9waXBl"},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseEncounterFilter(c.q); err == nil {
				t.Errorf("want error, got nil")
			}
		})
	}
}

func TestEncodeDecodeCursor_RoundTrip(t *testing.T) {
	sa := "2026-01-02T03:04:05.123456789Z"
	id := "ENC-abc123"
	c := encodeEncounterCursor(sa, id)
	gotSA, gotID, err := decodeEncounterCursor(c)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if gotSA != sa || gotID != id {
		t.Errorf("roundtrip mismatch: %q/%q", gotSA, gotID)
	}
}

func TestDecodeCursor_Empty(t *testing.T) {
	sa, id, err := decodeEncounterCursor("")
	if err != nil || sa != "" || id != "" {
		t.Errorf("empty cursor: sa=%q id=%q err=%v", sa, id, err)
	}
}

func TestDecodeCursor_Bad(t *testing.T) {
	if _, _, err := decodeEncounterCursor("!!!not-base64!!!"); err == nil {
		t.Errorf("want err for bad base64")
	}
	if _, _, err := decodeEncounterCursor("bm9waXBl"); err == nil { // "nopipe"
		t.Errorf("want err for no pipe")
	}
}

// B29.6.4a: order (asc|desc) в parseEncounterFilter.
func TestParseEncounterFilter_Order(t *testing.T) {
	cases := []struct {
		name, order, cursor string
		wantErr             bool
		wantOrder           string
	}{
		{"empty -> desc", "", "", false, encounterOrderDesc},
		{"asc", "asc", "", false, encounterOrderAsc},
		{"ASC (case-insens)", "ASC", "", false, encounterOrderAsc},
		{"Desc (case-insens)", "Desc", "", false, encounterOrderDesc},
		{"bogus", "xyz", "", true, ""},
		{"asc + cursor", "asc", "bm9waXBl", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := url.Values{}
			if c.order != "" {
				q.Set("order", c.order)
			}
			if c.cursor != "" {
				q.Set("cursor", c.cursor)
			}
			f, err := parseEncounterFilter(q)
			if c.wantErr {
				if err == nil {
					t.Errorf("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if f.Order != c.wantOrder {
				t.Errorf("order = %q, want %q", f.Order, c.wantOrder)
			}
		})
	}
}
