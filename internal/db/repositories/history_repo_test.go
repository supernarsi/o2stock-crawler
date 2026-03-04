package repositories

import (
	"reflect"
	"testing"
)

func TestUniqueUintIDs(t *testing.T) {
	tests := []struct {
		name string
		in   []uint
		want []uint
	}{
		{
			name: "empty",
			in:   nil,
			want: nil,
		},
		{
			name: "no duplicates",
			in:   []uint{1, 2, 3},
			want: []uint{1, 2, 3},
		},
		{
			name: "with duplicates keep first order",
			in:   []uint{3, 1, 3, 2, 1, 2},
			want: []uint{3, 1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uniqueUintIDs(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("uniqueUintIDs(%v)=%v, want=%v", tt.in, got, tt.want)
			}
		})
	}
}

func TestChunkUintIDs(t *testing.T) {
	tests := []struct {
		name      string
		in        []uint
		chunkSize int
		want      [][]uint
	}{
		{
			name:      "empty",
			in:        nil,
			chunkSize: 2,
			want:      nil,
		},
		{
			name:      "normal chunks",
			in:        []uint{1, 2, 3, 4, 5},
			chunkSize: 2,
			want:      [][]uint{{1, 2}, {3, 4}, {5}},
		},
		{
			name:      "invalid chunk size",
			in:        []uint{1, 2, 3},
			chunkSize: 0,
			want:      [][]uint{{1, 2, 3}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunkUintIDs(tt.in, tt.chunkSize)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("chunkUintIDs(%v, %d)=%v, want=%v", tt.in, tt.chunkSize, got, tt.want)
			}
		})
	}
}
