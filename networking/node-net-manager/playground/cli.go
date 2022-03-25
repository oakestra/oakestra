package playground

import (
	"NetManager/env"
	"NetManager/proxy"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"log"
	"net"
	"strconv"
	"strings"
)

var PUBLIC_ADDRESS = ""
var PUBLIC_PORT = 50103
var ENV *env.Environment
var PROXY proxy.GoProxyTunnel
var APP *tview.Application
var services [][]string
var entries [][]string
var killchan []*chan bool

func Cli_loop(addr string, port string) {
	services = make([][]string, 0)
	entries = make([][]string, 0)
	killchan = make([]*chan bool, 0)
	APP = tview.NewApplication()
	welcomeForm := tview.NewForm().
		AddInputField("Welcome to NetManagerPlayground", "!", 1, nil, nil).
		AddButton("Proceed", func() {
			APP.Stop()
			initEnv(addr, port)
		}).
		AddButton("Quit", func() {
			closePlayground(nil)
		})
	welcomeForm.SetBorder(true).SetTitle("P2P - Playground2Playground").SetTitleAlign(tview.AlignCenter)
	if err := APP.SetRoot(welcomeForm, true).Run(); err != nil {
		closePlayground(err)
	}
}

func initEnv(addr string, port string) {
	//initialize the proxy tunnel
	APP = tview.NewApplication()
	PUBLIC_PORT, _ = strconv.Atoi(port)
	PUBLIC_ADDRESS = addr
	PROXY = proxy.New()
	PROXY.Listen()
	cleanAll()

	//initialize the Env Manager
	config := env.Configuration{
		HostBridgeName:             "goProxyBridge",
		HostBridgeIP:               "172.19.0.0",
		HostBridgeMask:             "/26",
		HostTunName:                "goProxyTun",
		ConnectedInternetInterface: "",
		Mtusize:                    3000,
	}

	x := "0"
	y := "0"

	initForm := tview.NewForm().
		AddInputField("Input X and Y for the for the node subnetwork 172.19.x.y", "", 1, nil, nil).
		AddInputField("E.g. node1: 172.19.0.0 | node 2: 172.19.0.64", "", 1, nil, nil).
		AddDropDown("X", []string{"0", "1", "2", "3", "4"}, 0, func(option string, optionIndex int) {
			x = option
		}).
		AddDropDown("Y", []string{"0", "64", "128", "192"}, 0, func(option string, optionIndex int) {
			y = option
		}).
		AddInputField("MTU size", "3000", 5, nil, func(text string) {
			config.Mtusize, _ = strconv.Atoi(text)
		}).
		AddButton("Save", func() {
			APP.Stop()
			yint, _ := strconv.Atoi(y)
			config.HostBridgeIP = fmt.Sprintf("172.19.%s.%d", x, yint+1)
			ENV = env.NewCustom(PROXY.HostTUNDeviceName, config)
			PROXY.SetEnvironment(ENV)
			gotoMenu()
		}).
		AddButton("Quit", func() {
			closePlayground(nil)
		})
	initForm.SetBorder(true).SetTitle("Init node's playground").SetTitleAlign(tview.AlignCenter)
	if err := APP.SetRoot(initForm, true).SetFocus(initForm).Run(); err != nil {
		closePlayground(err)
	}
}

func gotoMenu() {
	APP = tview.NewApplication()
	list := tview.NewList().
		AddItem("Containers", "Check the currently deployed containers", 'a', func() {
			APP.Stop()
			listContainers()
		}).
		AddItem("Routes", "Show the configured routes", 'b', func() {
			APP.Stop()
			listRoutes()
		}).
		AddItem("Deploy container", "Deploy a new sample application connected to the NetManager", 'v', func() {
			APP.Stop()
			deployContainer()
		}).
		AddItem("Remove route", "Remove an existing route", 'x', func() {
			APP.Stop()
			removeRoutes()
		}).
		AddItem("Add table entry", "Add a new route towards an external service deployed on another playground", 'd', func() {
			APP.Stop()
			addTableEntry()
		}).
		AddItem("Undeploy container", "(not yet implemented) Undeploy a application that is currently running", 'e', nil).
		AddItem("Remove all containers", "Undeploy all running containers", 'f', func() {
			services = make([][]string, 0)
			for _, ch := range killchan {
				*ch <- true
			}
			cleanAll()
			killchan = make([]*chan bool, 0)
		}).
		AddItem("Quit", "Press to exit", 'q', func() {
			APP.Stop()
			closePlayground(nil)
		})
	if err := APP.SetRoot(list, true).SetFocus(list).Run(); err != nil {
		closePlayground(nil)
	}
}

func listContainers() {
	APP = tview.NewApplication()
	table := tview.NewTable().
		SetBorders(true)
	colsNames := strings.Split("index appname nsIP instanceIP RR_IP", " ")
	cols, rows := 5, len(services)+1
	word := 0
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			color := tcell.ColorWhite
			if r < 1 {
				color = tcell.ColorYellow
				table.SetCell(r, c,
					tview.NewTableCell(colsNames[word]).
						SetTextColor(color).
						SetAlign(tview.AlignCenter))
				word += 1
			} else {
				if c < 1 {
					table.SetCell(r, c,
						tview.NewTableCell(fmt.Sprintf("%d", r-1)).
							SetTextColor(color).
							SetAlign(tview.AlignCenter))
				} else {
					table.SetCell(r, c,
						tview.NewTableCell(services[r-1][c-1]).
							SetTextColor(color).
							SetAlign(tview.AlignCenter))
				}
			}
		}
	}
	table.Select(0, 0).SetFixed(1, 1).SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			APP.Stop()
			gotoMenu()
		}
		if key == tcell.KeyEnter {
			table.SetSelectable(true, true)
		}
	}).SetSelectedFunc(func(row int, column int) {
		table.GetCell(row, column).SetTextColor(tcell.ColorRed)
		table.SetSelectable(false, false)
	})
	if err := APP.SetRoot(table, true).SetFocus(table).Run(); err != nil {
		panic(err)
	}
}

func deployContainer() {
	APP = tview.NewApplication()
	sname := "test"
	image := "docker.io/curlimages/curl:7.82.0"
	instance := "0"
	cmd := ""
	iip := "172.30.10.10"
	sip := "172.30.20.20"
	form := tview.NewForm().
		AddInputField("Appname:", "test", 0, nil, func(text string) {
			sname = text
		}).
		AddInputField("Image url:", "docker.io/curlimages/curl:7.82.0", 0, nil, func(text string) {
			image = text
		}).
		AddDropDown("Instance", []string{"0", "1", "2", "3", "4", "5"}, 0, func(option string, optionIndex int) {
			instance = option
		}).
		AddInputField("Cmd", "ls", 0, nil, func(text string) {
			cmd = text
		}).
		AddInputField("InstanceIP: 172.30.", "10.10", 7, nil, func(text string) {
			iip = fmt.Sprintf("172.30.%s", text)
		}).
		AddInputField("RoundRobinIP (must match the other instances): 172.30.", "20.20", 7, nil, func(text string) {
			sip = fmt.Sprintf("172.30.%s", text)
		}).
		AddButton("Save", func() {
			APP.Stop()
			instanceint, _ := strconv.Atoi(instance)
			cmdArr := make([]string, 0)
			if len(cmd) > 0 {
				cmdArr = strings.Split(cmd, " ")
			}
			kill, addr, err := Start(sname, image, instanceint, cmdArr, iip, sip)
			if err != nil {
				fmt.Printf("%v", err)
				closePlayground(err)
			}
			app := []string{sname, addr, iip, sip}
			services = append(services, app)
			killchan = append(killchan, kill)
			gotoMenu()
		}).
		AddButton("Back", func() {
			APP.Stop()
			gotoMenu()
		})
	form.SetBorder(true).SetTitle("Enter some data").SetTitleAlign(tview.AlignCenter)
	if err := APP.SetRoot(form, true).SetFocus(form).Run(); err != nil {
		panic(err)
	}
}

func addTableEntry() {
	APP = tview.NewApplication()
	sname := "test"
	instance := "0"
	nodeip := "192.168.0.2"
	nodeport := "50103"
	nsip := "172.19.0.2"
	iip := "172.30.10.10"
	sip := "172.30.20.20"
	form := tview.NewForm().
		AddInputField("Appname:", "test", 0, nil, func(text string) {
			sname = text
		}).
		AddDropDown("Instance", []string{"0", "1", "2", "3", "4", "5"}, 0, func(option string, optionIndex int) {
			instance = option
		}).
		AddInputField("NamespaceIP: 172.19.", "0.2", 7, nil, func(text string) {
			nsip = fmt.Sprintf("172.19.%s", text)
		}).
		AddInputField("InstanceIP: 172.30.", "10.10", 7, nil, func(text string) {
			iip = fmt.Sprintf("172.30.%s", text)
		}).
		AddInputField("RoundRobinIP (must match the other instances): 172.30.", "20.20", 7, nil, func(text string) {
			sip = fmt.Sprintf("172.30.%s", text)
		}).
		AddInputField("Remote node IP: 192.168.", "0.2", 7, nil, func(text string) {
			nodeip = fmt.Sprintf("192.168.%s", text)
		}).
		AddInputField("Remote node port:", nodeport, 7, nil, func(text string) {
			nodeport = text
		}).
		AddButton("Save", func() {
			APP.Stop()
			instanceint, _ := strconv.Atoi(instance)
			portint, err := strconv.Atoi(nodeport)
			if err != nil {
				closePlayground(err)
			}
			ENV.AddTableQueryEntry(env.TableEntry{
				JobName:          sname,
				Appname:          sname,
				Appns:            "default",
				Servicename:      "test",
				Servicenamespace: "default",
				Instancenumber:   instanceint,
				Cluster:          0,
				Nodeip:           net.IP(nodeip),
				Nodeport:         portint,
				Nsip:             net.IP(nsip),
				ServiceIP: []env.ServiceIP{
					env.ServiceIP{
						IpType:  env.InstanceNumber,
						Address: net.IP(iip),
					},
					env.ServiceIP{
						IpType:  env.RoundRobin,
						Address: net.IP(sip),
					},
				},
			})
			entries = append(entries, strings.Split(fmt.Sprintf("%s %s %s %s %s %s", sname, nsip, iip, sip, nodeip, nodeport), " "))
			gotoMenu()
		}).
		AddButton("Back", func() {
			APP.Stop()
			gotoMenu()
		})
	form.SetBorder(true).SetTitle("Enter some data").SetTitleAlign(tview.AlignCenter)
	if err := APP.SetRoot(form, true).SetFocus(form).Run(); err != nil {
		panic(err)
	}
}

func listRoutes() {
	APP = tview.NewApplication()
	table := tview.NewTable().
		SetBorders(true)
	colsNames := strings.Split("index appname nsIP instanceIP RR_IP nodeIP port", " ")
	cols, rows := 7, len(entries)+1
	word := 0
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			color := tcell.ColorWhite
			if r < 1 {
				color = tcell.ColorYellow
				table.SetCell(r, c,
					tview.NewTableCell(colsNames[word]).
						SetTextColor(color).
						SetAlign(tview.AlignCenter))
				word += 1
			} else {
				if c < 1 {
					table.SetCell(r, c,
						tview.NewTableCell(fmt.Sprintf("%d", r-1)).
							SetTextColor(color).
							SetAlign(tview.AlignCenter))
				} else {
					table.SetCell(r, c,
						tview.NewTableCell(entries[r-1][c-1]).
							SetTextColor(color).
							SetAlign(tview.AlignCenter))
				}
			}
		}
	}
	table.Select(0, 0).SetFixed(1, 1).SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			APP.Stop()
			gotoMenu()
		}
		if key == tcell.KeyEnter {
			table.SetSelectable(true, false)
		}
	})
	if err := APP.SetRoot(table, true).SetFocus(table).Run(); err != nil {
		panic(err)
	}
}

func removeRoutes() {
	APP = tview.NewApplication()
	table := tview.NewTable().
		SetBorders(true)
	colsNames := strings.Split("index appname nsIP instanceIP RR_IP nodeIP port", " ")
	cols, rows := 7, len(entries)+1
	word := 0
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			color := tcell.ColorWhite
			if r < 1 {
				color = tcell.ColorYellow
				table.SetCell(r, c,
					tview.NewTableCell(colsNames[word]).
						SetTextColor(color).
						SetAlign(tview.AlignCenter))
				word += 1
			} else {
				if c < 1 {
					table.SetCell(r, c,
						tview.NewTableCell(fmt.Sprintf("%d", r-1)).
							SetTextColor(color).
							SetAlign(tview.AlignCenter))
				} else {
					table.SetCell(r, c,
						tview.NewTableCell(entries[r-1][c-1]).
							SetTextColor(color).
							SetAlign(tview.AlignCenter))
				}
			}
		}
	}
	table.Select(0, 0).SetFixed(1, 1).SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			APP.Stop()
			gotoMenu()
		}
		if key == tcell.KeyEnter {
			table.SetSelectable(true, false)
		}
	}).SetSelectedFunc(func(row int, column int) {
		if row > 0 {
			APP.Stop()
			ENV.RemoveNsIPEntries(entries[row-1][1])
			entries = append(entries[:row-1], entries[row:]...)
			listRoutes()
		}
	})
	if err := APP.SetRoot(table, true).SetFocus(table).Run(); err != nil {
		panic(err)
	}
}

func closePlayground(err error) {
	APP.Stop()
	cleanAll()
	log.Fatalln(err)
}
