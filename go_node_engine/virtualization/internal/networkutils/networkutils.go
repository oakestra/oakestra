package networkutils

import (
	"errors"
	"net"
	rt "runtime"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

func RetrieveTapMacInNamespace(namespace string) (string, error) {
	var mac string
	err := execInsideNsByName(namespace, func() error {
		link, err := netlink.LinkByName("tap0")
		if err != nil {
			return err
		}
		mac = link.Attrs().HardwareAddr.String()
		return nil
	})
	return mac, err
}

// delete and returns the defaultIp Gateway and Netmask of a given namespace
func DeleteDefaultIpGwMask(namespace string) (string, string, string, string, error) {
	defaultRouteFilter := &netlink.Route{Dst: nil}
	ip, gw, mask, mac := "", "", "", ""

	err := execInsideNsByName(namespace, func() error {

		routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, defaultRouteFilter, netlink.RT_FILTER_DST)
		if err != nil {
			return err
		}
		if n := len(routes); n > 1 {
			return err
		}
		if len(routes) == 0 {
			return err
		}
		route := &routes[0]

		routeIdx := route.LinkIndex
		routelink, err := netlink.LinkByIndex(routeIdx)
		if err != nil {
			return err
		}

		macvtap, err := netlink.LinkByName("tap0")
		if err != nil {
			return err
		}

		addrs, err := netlink.AddrList(routelink, netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		if len(addrs) == 0 {
			return errors.New("no IP address found")
		}

		ip = addrs[0].IP.String()
		gw = route.Gw.String()
		mac = macvtap.Attrs().HardwareAddr.String()
		mask = net.IP(addrs[0].Mask).String()

		if err = netlink.AddrDel(routelink, &addrs[0]); err != nil {
			return err
		}

		return nil
	})

	return ip, gw, mask, mac, err
}

// Execute function inside a namespace based on Ns name
func execInsideNsByName(Nsname string, function func() error) error {
	var containerNs netns.NsHandle

	rt.LockOSThread()
	defer rt.UnlockOSThread()

	stdNetns, err := netns.Get()
	if err == nil {
		defer func() {
			_ = stdNetns.Close()
		}()
		containerNs, err = netns.GetFromName(Nsname)
		if err == nil {
			defer func() {
				_ = netns.Set(stdNetns)
			}()
			err = netns.Set(containerNs)
			if err == nil {
				err = function()
			}
		}
	}
	return err
}
