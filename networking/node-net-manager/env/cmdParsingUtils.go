package env

import (
	"strconv"
	"strings"
)

// given the result of the ip netns list command, gives back the list of all the network namespaces
func NetworkNamespacesList(list string) []string {
	namespaceList := make([]string, 0)
	lines := strings.Split(list, "\n")
	for _, line := range lines {
		namespaceName := strings.Split(line, " ")[0]
		if namespaceName != " " && namespaceName != "" {
			namespaceList = append(namespaceList, namespaceName)
		}
	}
	return namespaceList
}

// given the result of the ip list command, gives back the list of networkInterface
func NetworkVethLinkList(list string) []networkInterface {
	interfaceList := make([]networkInterface, 0)
	lines := strings.Split(list, "\n")
	for _, line := range lines {
		line = strings.Replace(line, ":", "", -1)
		keywords := strings.Split(line, " ")
		//keyword[0] should be the current interface number
		currentInterfaceNumber, err := strconv.Atoi(keywords[0])
		if err == nil {
			//keyword[1] should be a couple like veth1@if4
			interfaceName := keywords[1]
			//if is a veth we save the link
			if strings.Contains(interfaceName, "veth0") {
				//since veth are always created in couple find out the links
				links := strings.Split(interfaceName, "@")
				//build the return struct
				isInterface := false
				veth1 := links[1]
				interfaceNumber := -1
				if !strings.Contains(links[1], "veth") {
					isInterface = true
					interfaceNumber, _ = strconv.Atoi(strings.Replace(links[1], "if", "", -1))
					veth1 = ""
				}
				interfaceList = append(interfaceList, networkInterface{
					number:                   currentInterfaceNumber,
					veth0:                    links[0],
					veth1:                    veth1,
					isConnectedToAnInterface: isInterface,
					interfaceNumber:          interfaceNumber,
				})
			}
		}
	}
	return interfaceList
}

// given the result of the ip list command , gives back the list of the interface names
func extractNetInterfaceName(list string) []string {
	result := make([]string, 0)

	lines := strings.Split(list, "\n")
	for _, line := range lines {
		line = strings.Replace(line, ":", "", -1)
		keywords := strings.Split(line, " ")
		//keyword[0] should be the current interface number
		_, err := strconv.Atoi(keywords[0])
		if err == nil {
			//append bridge name
			result = append(result, keywords[1])
		}
	}

	return result
}
