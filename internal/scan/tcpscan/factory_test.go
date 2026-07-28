package tcpscan

import "testing"

func TestNewFactory(t *testing.T) {
	if s, err := New(ModeConnect, Options{}); err != nil || s.Mode() != ModeConnect {
		t.Errorf("connect 工厂失败: mode=%v err=%v", s, err)
	}
	if s, err := New(ModeSyn, Options{}); err != nil || s.Mode() != ModeSyn {
		t.Errorf("syn 工厂失败: mode=%v err=%v", s, err)
	}
	if s, err := New(ModeAck, Options{}); err != nil || s.Mode() != ModeAck {
		t.Errorf("ack 工厂失败: mode=%v err=%v", s, err)
	}
	if _, err := New(Mode("bogus"), Options{}); err == nil {
		t.Error("非法模式应返回 error")
	}
}
