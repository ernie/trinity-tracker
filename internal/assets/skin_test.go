package assets

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseSkinMap(t *testing.T) {
	in := "tag_head,\nh_cigar,models/players/sarge/cigar.tga\nh_head,models/players/sarge/band.tga\n"
	got, err := ParseSkinMap(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []SkinEntry{
		{Surface: "h_cigar", Texture: "models/players/sarge/cigar.tga"},
		{Surface: "h_head", Texture: "models/players/sarge/band.tga"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSkinMap = %v, want %v", got, want)
	}
}
