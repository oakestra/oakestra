package gpu

import "testing"

func TestParseQueryAllMultipleGPUs(t *testing.T) {
	out := "0, 24576, 1000, 23576, 10, 40, 550.54.14\n1, 24576, 20000, 4576, 90, 70, 550.54.14\n"
	got := ParseQueryAll(out)
	if len(got) != 2 {
		t.Fatalf("expected 2 GPUs, got %d", len(got))
	}
	want := Info{Index: 1, MemTotal: 24576, MemUsed: 20000, MemFree: 4576, Utilization: 90, Temperature: 70, DriverVersion: "550.54.14"}
	if got[1] != want {
		t.Errorf("got %+v, want %+v", got[1], want)
	}
}

func TestParseQueryAllKeepsNvidiaSmiOrder(t *testing.T) {
	out := "0, 100, 0, 10, 0, 30, 1\n1, 100, 0, 500, 0, 30, 1\n2, 100, 0, 50, 0, 30, 1\n"
	got := ParseQueryAll(out)
	if len(got) != 3 || got[0].MemFree != 10 || got[1].MemFree != 500 || got[2].MemFree != 50 {
		t.Errorf("unexpected result %+v", got)
	}
}

func TestParseQueryAllNotAvailableFields(t *testing.T) {
	got := ParseQueryAll("0, 8192, [N/A], [N/A], [N/A], [N/A], [N/A]\n")
	if len(got) != 1 {
		t.Fatalf("expected 1 GPU, got %d", len(got))
	}
	want := Info{Index: 0, MemTotal: 8192}
	if got[0] != want {
		t.Errorf("got %+v, want %+v", got[0], want)
	}
}

func TestParseQueryAllEmpty(t *testing.T) {
	for _, in := range []string{"", "\n", "  \n\n"} {
		if got := ParseQueryAll(in); got == nil || len(got) != 0 {
			t.Errorf("input %q: expected empty non-nil slice, got %#v", in, got)
		}
	}
}

func TestParseQueryAllMalformedLine(t *testing.T) {
	out := "garbage\n0, 100, 10, 90\nx, 100, 10, 90, 1, 2, 3\n1, 200, 20, 180, 5, 50, 535.1\n"
	got := ParseQueryAll(out)
	if len(got) != 1 || got[0].Index != 1 || got[0].MemFree != 180 {
		t.Errorf("expected only the valid line to survive, got %+v", got)
	}
}
