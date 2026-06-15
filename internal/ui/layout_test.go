package ui

import "testing"

func TestCalcLayoutOrdering(t *testing.T) {
	// Across a range of sizes, the row landmarks must stay in their fixed
	// top-to-bottom order: verRow < artTop <= artBottom < sep1 < sep2 < workRow.
	sizes := []struct{ w, h int }{
		{80, 25}, {120, 40}, {200, 60}, {40, 15}, {100, 30},
	}
	for _, s := range sizes {
		for _, msg := range []int{0, 5, 20} {
			ly := CalcLayout(s.w, s.h, 48, msg)
			if !(ly.VerRow < ly.ArtTop &&
				ly.ArtTop <= ly.ArtBottom &&
				ly.ArtBottom <= ly.Sep1 &&
				ly.Sep1 < ly.Sep2 &&
				ly.Sep2 <= ly.WorkRow) {
				t.Errorf("size %dx%d msg=%d: row landmarks out of order: %+v", s.w, s.h, msg, ly)
			}
			if ly.InputW < 1 || ly.TextW < 1 {
				t.Errorf("size %dx%d: input widths must stay positive: %+v", s.w, s.h, ly)
			}
		}
	}
}

func TestCalcLayoutGoldenNoMsgs(t *testing.T) {
	ly := CalcLayout(80, 25, 48, 0)
	want := Layout{
		Inner: 78, Rows: 23,
		InputW: 46, TextW: 44,
		BottomGap: 5,
		WorkRow:   22, Sep2: 16, Sep1: 12,
		ArtBottom: 8, ArtTop: 6,
		VerRow: 5, HintRow: 19,
		ArtPad: 15,
	}
	if ly.Inner != want.Inner || ly.Rows != want.Rows ||
		ly.InputW != want.InputW || ly.TextW != want.TextW ||
		ly.WorkRow != want.WorkRow || ly.Sep2 != want.Sep2 || ly.Sep1 != want.Sep1 ||
		ly.ArtBottom != want.ArtBottom || ly.ArtTop != want.ArtTop ||
		ly.VerRow != want.VerRow || ly.HintRow != want.HintRow ||
		ly.ArtPad != want.ArtPad {
		t.Errorf("CalcLayout(80,25,48,0):\n got  %+v\n want approximate %+v", ly, want)
	}
	if ly.MsgRows != 0 {
		t.Errorf("msgRows should be 0 with no messages, got %d", ly.MsgRows)
	}
}

func TestCalcLayoutGoldenWithMsgs(t *testing.T) {
	ly := CalcLayout(80, 25, 48, 8)
	if ly.VerRow != 2 {
		t.Errorf("verRow should be 2 with messages, got %d", ly.VerRow)
	}
	if ly.MsgTop != 7 {
		t.Errorf("msgTop should be 7, got %d", ly.MsgTop)
	}
	// Input block is pinned to the bottom: workRow=rows-1, sep2=workRow-1,
	// sep1=sep2-InputRows-1. Messages fill msgTop..sep1.
	if ly.WorkRow != 22 {
		t.Errorf("workRow should be 22, got %d", ly.WorkRow)
	}
	if ly.Sep2 != 21 {
		t.Errorf("sep2 should be 21, got %d", ly.Sep2)
	}
	if ly.Sep1 != 17 {
		t.Errorf("sep1 should be 17, got %d", ly.Sep1)
	}
	if ly.MsgRows != 10 {
		t.Errorf("msgRows should be 10 (sep1-msgTop), got %d", ly.MsgRows)
	}
	if ly.EdgeX != 2 {
		t.Errorf("edgeX should be 2 in messages mode, got %d", ly.EdgeX)
	}
}

func TestCalcLayoutTinyClamps(t *testing.T) {
	ly := CalcLayout(1, 1, 48, 0)
	if ly.Inner < 1 || ly.Rows < 1 || ly.InputW < 1 || ly.TextW < 1 {
		t.Errorf("tiny terminal should clamp to positive dims: %+v", ly)
	}
}
