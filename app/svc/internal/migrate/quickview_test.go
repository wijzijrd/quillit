package migrate

import "testing"

func TestQuickViewCard_EmitsBoldLabelLines(t *testing.T) {
	fields := map[string]string{"Role": "innkeeper", "Location": "Waterdeep"}
	got := QuickViewCard("characters", fields)
	want := ":::card characters\n**Location:** Waterdeep\n**Role:** innkeeper\n:::"
	if got != want {
		t.Errorf("QuickViewCard() =\n%q\nwant\n%q", got, want)
	}
}

func TestQuickViewCard_SkipsEmptyValues(t *testing.T) {
	fields := map[string]string{"Role": "innkeeper", "Nickname": ""}
	got := QuickViewCard("characters", fields)
	want := ":::card characters\n**Role:** innkeeper\n:::"
	if got != want {
		t.Errorf("QuickViewCard() with an empty field =\n%q\nwant\n%q", got, want)
	}
}

func TestQuickViewCard_EmptyFieldsProducesEmptyString(t *testing.T) {
	if got := QuickViewCard("characters", map[string]string{}); got != "" {
		t.Errorf("QuickViewCard() with no fields = %q, want empty string", got)
	}
	if got := QuickViewCard("characters", map[string]string{"Role": ""}); got != "" {
		t.Errorf("QuickViewCard() with only empty fields = %q, want empty string", got)
	}
}
