package network

import (
	"net"
	"testing"

	"gotest.tools/assert"
)

func TestPortMappingEmpty(t *testing.T) {
	mock := &mockiptable{}
	iptable = mock
	// empty string
	err := ManageContainerPorts(net.ParseIP("0.0.0.0"), "", OpenPorts)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(mock.CalledWith), 0)
}

func TestPortMappingUdp(t *testing.T) {
	mock := &mockiptable{}
	iptable = mock
	// udp 80
	err := ManageContainerPorts(net.ParseIP("0.0.0.0"), "80:80/udp", OpenPorts)
	if err != nil {
		t.Fatal(err)
	}
	for i, arg := range []string{"nat", chain, "-p", "udp", "--dport", "80", "-j", "DNAT", "--to-destination", "0.0.0.0:80"} {
		assert.Equal(t, arg, mock.CalledWith[i])
	}
	// udp 80 and 90
	err = ManageContainerPorts(net.ParseIP("0.0.0.0"), "80:80/udp;90:100/udp", OpenPorts)
	if err != nil {
		t.Fatal(err)
	}
	for i, arg := range []string{"nat", chain, "-p", "udp", "--dport", "90", "-j", "DNAT", "--to-destination", "0.0.0.0:100"} {
		assert.Equal(t, arg, mock.CalledWith[i])
	}
}

func TestPortMappingTcp(t *testing.T) {
	mock := &mockiptable{}
	iptable = mock
	// tcp 80
	err := ManageContainerPorts(net.ParseIP("0.0.0.0"), "80", OpenPorts)
	if err != nil {
		t.Fatal(err)
	}
	for i, arg := range []string{"nat", chain, "-p", "tcp", "--dport", "80", "-j", "DNAT", "--to-destination", "0.0.0.0:80"} {
		assert.Equal(t, arg, mock.CalledWith[i])
	}
	// tcp 80:80
	err = ManageContainerPorts(net.ParseIP("0.0.0.0"), "80:80", OpenPorts)
	if err != nil {
		t.Fatal(err)
	}
	for i, arg := range []string{"nat", chain, "-p", "tcp", "--dport", "80", "-j", "DNAT", "--to-destination", "0.0.0.0:80"} {
		assert.Equal(t, arg, mock.CalledWith[i])
	}
	// tcp 80 and 90
	err = ManageContainerPorts(net.ParseIP("0.0.0.0"), "80:80/tcp;90:100/tcp", OpenPorts)
	if err != nil {
		t.Fatal(err)
	}
	for i, arg := range []string{"nat", chain, "-p", "tcp", "--dport", "90", "-j", "DNAT", "--to-destination", "0.0.0.0:100"} {
		assert.Equal(t, arg, mock.CalledWith[i])
	}
}

func TestPortMappingInvalid(t *testing.T) {
	mock := &mockiptable{}
	iptable = mock

	err := ManageContainerPorts(net.ParseIP("0.0.0.0"), "80:80-80", OpenPorts)
	if err == nil {
		t.Fatal("80:80-80 must be invalid")
	}

	err = ManageContainerPorts(net.ParseIP("0.0.0.0"), " ", OpenPorts)
	if err == nil {
		t.Fatal("space must be invalid")
	}

	err = ManageContainerPorts(net.ParseIP("0.0.0.0"), "hello", OpenPorts)
	if err == nil {
		t.Fatal("hello must be invalid")
	}
}

func TestIncIP_simple(t *testing.T) {
	ip1 := []byte{0, 0, 0, 2}

	inc := NextIPv4(ip1, 1)
	want := []byte{0, 0, 0, 3}

	if !inc.Equal(want) {
		t.Fatal("Problem in NextIP function.")
	}
}

type mockiptable struct {
	CalledWith []string
}

func (t *mockiptable) Append(s string, s2 string, s3 ...string) error {
	t.CalledWith = append([]string{s, s2}, s3...)
	return nil
}

func (t *mockiptable) AppendUnique(s string, s2 string, s3 ...string) error {
	// TODO implement me
	panic("implement me")
}

func (t *mockiptable) Delete(s string, s2 string, s3 ...string) error {
	// TODO implement me
	panic("implement me")
}

func (t *mockiptable) DeleteChain(s string, s2 string) error {
	// TODO implement me
	panic("implement me")
}

func (t *mockiptable) AddChain(s string, s2 string) error {
	// TODO implement me
	panic("implement me")
}
