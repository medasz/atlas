package tcpscan

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name        string
		mode        Mode
		flags       uint8
		icmpUnreach bool
		want        State
	}{
		// SYN / connect：四态齐全
		{"syn/open", ModeSyn, tcpSYN | tcpACK, false, Open},
		{"syn/closed", ModeSyn, tcpRST, false, Closed},
		{"syn/filtered", ModeSyn, 0, true, Filtered},
		{"syn/timeout", ModeSyn, 0, false, Timeout},
		{"connect/open", ModeConnect, tcpSYN | tcpACK, false, Open},
		{"connect/closed", ModeConnect, tcpRST, false, Closed},
		{"connect/filtered", ModeConnect, 0, true, Filtered},
		{"connect/timeout", ModeConnect, 0, false, Timeout},

		// ACK：仅 unfiltered / filtered
		{"ack/unfiltered", ModeAck, tcpRST, false, Unfiltered},
		{"ack/filtered(icmp)", ModeAck, 0, true, Filtered},
		{"ack/filtered(timeout)", ModeAck, 0, false, Filtered},

		// FIN / Null / Xmas：closed / filtered / open|filtered
		{"fin/closed", ModeFin, tcpRST, false, Closed},
		{"fin/filtered", ModeFin, 0, true, Filtered},
		{"fin/open|filtered", ModeFin, 0, false, OpenFiltered},
		{"null/open|filtered", ModeNull, 0, false, OpenFiltered},
		{"xmas/closed", ModeXmas, tcpRST, false, Closed},
		{"xmas/filtered", ModeXmas, 0, true, Filtered},
		{"xmas/open|filtered", ModeXmas, 0, false, OpenFiltered},

		// 边界：FIN 收到异常 SYN-ACK（无任何 RST/ICMP）→ 归为 open|filtered
		{"fin/anomaly->open|filtered", ModeFin, tcpSYN | tcpACK, false, OpenFiltered},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classify(c.flags, c.icmpUnreach, c.mode)
			if got != c.want {
				t.Errorf("classify(%s, flags=0x%02x, icmp=%v) = %q, want %q",
					c.mode, c.flags, c.icmpUnreach, got, c.want)
			}
		})
	}
}

func TestModeFlags(t *testing.T) {
	expect := map[Mode]uint8{
		ModeSyn:  tcpSYN,
		ModeFin:  tcpFIN,
		ModeNull: 0,
		ModeXmas: tcpFIN | tcpPSH | tcpURG,
		ModeAck:  tcpACK,
	}
	for m, want := range expect {
		if got := m.flags(); got != want {
			t.Errorf("%s.flags() = 0x%02x, want 0x%02x", m, got, want)
		}
	}
}

func TestParseMode(t *testing.T) {
	if m, err := ParseMode("syn"); err != nil || m != ModeSyn {
		t.Errorf("ParseMode(syn) = %q, %v", m, err)
	}
	if _, err := ParseMode("bogus"); err == nil {
		t.Error("ParseMode(bogus) expected error")
	}
	if ModeConnect.IsRaw() {
		t.Error("connect should not be raw")
	}
	if !ModeSyn.IsRaw() {
		t.Error("syn should be raw")
	}
}
