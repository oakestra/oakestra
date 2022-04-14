package network

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"tailscale.com/net/interfaces"
	"time"
)

// GetLocalIP returns the non loopback local IP of the host and the associated interface
func GetLocalIPandIface() (string, string) {
	list, err := net.Interfaces()
	if err != nil {
		log.Printf("not net Interfaces found")
		panic(err)
	}
	defaultIfce, err := interfaces.DefaultRouteInterface()
	if err != nil {
		log.Printf("not default Interfaces found")
		panic(err)
	}

	for _, iface := range list {
		addrs, err := iface.Addrs()
		if err != nil {
			panic(err)
		}
		for _, address := range addrs {
			// check the address type and if it is not a loopback the display it
			if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && iface.Name == defaultIfce {
				if ipnet.IP.To4() != nil {
					log.Println("Local Interface in use: ", iface.Name, " with addr ", ipnet.IP.String())
					return ipnet.IP.String(), iface.Name
				}
			}
		}
	}

	return "", ""
}

func NameUniqueHash(name string, size int) string {
	shaHashFunc := sha1.New()
	shaHashFunc.Write([]byte(fmt.Sprintf("%s,%s", time.Now().String(), name)))
	hashed := shaHashFunc.Sum(nil)
	for size > len(hashed) {
		hashed = append(hashed, hashed...)
	}
	hashedAndEncoded := base64.URLEncoding.EncodeToString(hashed)
	return hashedAndEncoded[:size]
}

//Given an ipv4, gives the next IP
func NextIP(ip net.IP, inc uint) net.IP {
	i := ip.To4()
	v := uint(i[0])<<24 + uint(i[1])<<16 + uint(i[2])<<8 + uint(i[3])
	v += inc
	v3 := byte(v & 0xFF)
	v2 := byte((v >> 8) & 0xFF)
	v1 := byte((v >> 16) & 0xFF)
	v0 := byte((v >> 24) & 0xFF)
	return net.IPv4(v0, v1, v2, v3)
}
