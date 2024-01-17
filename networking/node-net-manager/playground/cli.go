package playground

import (
	"NetManager/env"
	"NetManager/proxy"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var PUBLIC_ADDRESS = ""
var LISTEN_PORT = "6000"
var PUBLIC_PORT = 50103
var ENV *env.Environment
var PROXY proxy.GoProxyTunnel
var APP *tview.Application
var Services [][]string
var Entries [][]string
var killchan []*chan bool

func CliLoop(addr string, port string) {
	Services = make([][]string, 0)
	Entries = make([][]string, 0)
	killchan = make([]*chan bool, 0)
	PUBLIC_ADDRESS = addr
	PUBLIC_PORT, _ = strconv.Atoi(port)
	APP = tview.NewApplication()
	welcomeForm := tview.NewForm().
		AddInputField("Welcome to NetManagerPlayground, node address:", PUBLIC_ADDRESS, 0, nil, func(text string) {
			PUBLIC_ADDRESS = text
		}).
		AddButton("Proceed", func() {
			APP.Stop()
			initEnv(addr, port)
		}).
		AddButton("Quit", func() {
			closePlayground(nil)
		})
	welcomeForm.SetBorder(true).SetTitle("P2P - Playground2Playground").SetTitleAlign(tview.AlignCenter)
	if err := APP.SetRoot(welcomeForm, true).SetFocus(welcomeForm).Run(); err != nil {
		closePlayground(err)
	}
}

func initEnv(addr string, port string) {
	//initialize the proxy tunnel
	APP = tview.NewApplication()
	PROXY = proxy.New()
	PROXY.Listen()
	cleanAll()

	//initialize the Env Manager
	config := env.Configuration{
		HostBridgeName:             "goProxyBridge",
		HostBridgeIP:               "10.19.0.0",
		HostBridgeMask:             "/26",
		HostTunName:                "goProxyTun",
		ConnectedInternetInterface: "",
		Mtusize:                    1450,
	}

	x := "0"
	y := "0"

	initForm := tview.NewForm().
		AddInputField("Input X and Y for the for the node subnetwork 10.19.x.y", "", 1, nil, nil).
		AddInputField("E.g. node1: 10.19.0.0 | node 2: 10.19.0.64", "", 1, nil, nil).
		AddDropDown("X", []string{"0", "1", "2", "3", "4"}, 0, func(option string, optionIndex int) {
			x = option
		}).
		AddDropDown("Y", []string{"0", "64", "128", "192"}, 0, func(option string, optionIndex int) {
			y = option
		}).
		AddInputField("P2P sync public port", LISTEN_PORT, 5, nil, func(text string) {
			LISTEN_PORT = text
		}).
		AddInputField("MTU size", "1450", 5, nil, func(text string) {
			config.Mtusize, _ = strconv.Atoi(text)
		}).
		AddButton("Save", func() {
			APP.Stop()
			yint, _ := strconv.Atoi(y)
			config.HostBridgeIP = fmt.Sprintf("10.19.%s.%d", x, yint+1)
			ENV = env.NewCustom(PROXY.HostTUNDeviceName, config)
			env.InitUnikernelDeployment(ENV)
			env.InitContainerDeployment(ENV)
			PROXY.SetEnvironment(ENV)
			go HandleHttpSyncRequests(LISTEN_PORT)
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
		AddItem("P2P sync", "Sync routes with another node's p2p instance", 'd', func() {
			APP.Stop()
			p2pSync()
		}).
		AddItem("Undeploy container", "(not yet implemented) Undeploy a application that is currently running", 'e', nil).
		AddItem("Remove all containers", "Undeploy all running containers", 'f', func() {
			Services = make([][]string, 0)
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
		closePlayground(err)
	}
}

func listContainers() {
	APP = tview.NewApplication()
	table := tview.NewTable().
		SetBorders(true)
	colsNames := strings.Split("index appname nsIP instanceIP RR_IP", " ")
	cols, rows := 5, len(Services)+1
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
						tview.NewTableCell(Services[r-1][c-1]).
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
	cmd := "sh -c tail -f /dev/null"
	iip := "10.30.10.10"
	sip := "10.30.20.20"
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
		AddInputField("Cmd", cmd, 0, nil, func(text string) {
			cmd = text
		}).
		AddInputField("InstanceIP: 10.30.", "10.10", 7, nil, func(text string) {
			iip = fmt.Sprintf("10.30.%s", text)
		}).
		AddInputField("RoundRobinIP (must match the other instances): 10.30.", "20.20", 7, nil, func(text string) {
			sip = fmt.Sprintf("10.30.%s", text)
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
			Services = append(Services, app)
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

func p2pSync() {
	APP = tview.NewApplication()
	address := "192.168.0.2"
	port := "6000"
	form := tview.NewForm().
		AddInputField("P2P Node address: ", address, 7, nil, func(text string) {
			address = text
		}).
		AddInputField("P2P Node port", "6000", 7, nil, func(text string) {
			port = text
		}).
		AddButton("Sync", func() {
			APP.Stop()
			err := AskSync(address, port, Entries)
			if err != nil {
				log.Printf("ERROR: impossible to sync: %v", err)
			}
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
	cols, rows := 7, len(Entries)+1
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
						tview.NewTableCell(Entries[r-1][c-1]).
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
	cols, rows := 7, len(Entries)+1
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
						tview.NewTableCell(Entries[r-1][c-1]).
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
			ENV.RemoveNsIPEntries(Entries[row-1][1])
			Entries = append(Entries[:row-1], Entries[row:]...)
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

func periodic_redraw() {
	select {
	case <-time.After(time.Second * 2):
		APP.Draw()
	}
}
