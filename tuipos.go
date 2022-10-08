package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/gorilla/websocket"
	tm "github.com/nsf/termbox-go"
	"github.com/tidwall/gjson"
	"golang.org/x/exp/slices"
)

// variables pour la connexion websocket
var (
	host = flag.String("host", "localhost", "Host Ip adress")
	port = flag.String("port", "3333", "Port")
)

// variables utilisées par l'UI
var (
	LINE_SIZE           = 56
	posInput            = ""
	terminal_output_var = ""
	lines               []string
	mode                = ""
)

// état du Moteur
var (
	ENGINE_MODE     int64 = 0
	SCO_SUPERVISION       = false
)

type DataStore struct {
	posDisplay      []string
	customerDisplay []string
	receipt         []string
	changed         bool
}

func (self *DataStore) setPosDisplay(v []string) {
	if !slices.Equal(v, self.posDisplay) {
		self.changed = true
		self.posDisplay = []string{"", ""}
		for i, val := range v {
			if i > 2 {
				break
			}
			self.posDisplay[i] = val
		}
	}
}

func (self *DataStore) setCustomerDisplay(v []string) {
	if !slices.Equal(v, self.customerDisplay) {
		self.changed = true
		self.customerDisplay = []string{"", ""}
		for i, val := range v {
			if i > 2 {
				break
			}
			self.customerDisplay[i] = val
		}
	}
}

func (self *DataStore) setReceipt(v []string) {
	if !slices.Equal(v, self.receipt) {
		self.changed = true
		self.receipt = v
	}
}

func (self *DataStore) hasChanged() bool {
	return self.changed
}

func (self *DataStore) updated() {
	self.changed = false
}

var (
	apiConnection *websocket.Conn
	apiConnected  = false
)

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func center(s string, n int, fill string) string {
	if len(s) >= n {
		return s
	}
	div := (n - len(s)) / 2
	space := ""
	if (2*div + len(s)) < n { // trailing space
		space = " "
	}

	return strings.Repeat(fill, div) + s + strings.Repeat(fill, div) + space
}

func updateInput() {
	//POS input
	p := widgets.NewParagraph()
	p.Title = "POS input"
	p.Text = posInput
	p.SetRect(0, 4, 32, 7)
	ui.Render(p)
}

func updateMode() {
	//POS input
	p := widgets.NewParagraph()
	p.Title = "SCO display"
	p.Text = mode
	p.SetRect(34, 4, 58, 7)
	ui.Render(p)

}

func decode(ds *DataStore, msg string, forceRefresh bool) {
	// decode_mu.Lock() // only one caller at a time
	// defer decode_mu.Unlock()

	_, h := tm.Size()
	h = max(h, 12+len(lines))
	LINE_COUNT := h - 4 - 2 // screen - display - border

	iderr := gjson.Get(msg, "iderr").String()
	_key := gjson.Get(msg, "_key").String()

	if _key == "badge" && iderr == "0" {
		mode = "Badge accepted"
	} else if _key == "badge" && iderr != "0" {
		mode = "Badge error"
	} else if _key == "init" {
		mode = "init"
	}
	updateMode()

	value := gjson.Get(msg, "seqb")
	seqb := value.Uint()
	value = gjson.Get(msg, "seqe")
	seqe := value.Uint()

	visuCaiChanged := false
	visuCliChanged := false
	receiptChanged := false

	visuCai := []string{"", ""}
	visuCli := []string{"", ""}
	var tkt []string

	for i := seqb; i <= seqe; i++ {
		d := gjson.Get(msg, strconv.Itoa(int(i)))
		d.ForEach(func(key, value gjson.Result) bool {
			switch key.String() {
			case "VCS1":
				visuCai[0] = value.String()
				visuCaiChanged = true
			case "VCS2":
				visuCai[1] = value.String()
				visuCaiChanged = true
			case "VCC1":
				visuCli[0] = value.String()
				visuCliChanged = true
			case "VCC2":
				visuCli[1] = value.String()
				visuCliChanged = true
			case "VISUTKT":
				lines := value.Array()
				for _, l := range lines[max(0, len(lines)-LINE_COUNT):] {
					txt := gjson.Get(l.String(), "txt")
					tkt = append(tkt, center(txt.String(), LINE_SIZE, " "))
				}
				receiptChanged = true
			case "typtpv":
				ENGINE_MODE = value.Int()

			default:
			}
			return true // keep iterating
		})
	}

	if visuCaiChanged {
		ds.setPosDisplay(visuCai)
	}

	if visuCliChanged {
		ds.setCustomerDisplay(visuCli)
	}

	if receiptChanged {
		ds.setReceipt(tkt)
	}

	if !forceRefresh && !ds.hasChanged() { // screen not changed, nothing todo
		return
	}

	ds.updated()

	help := []string{
		"a abort",
		"A arrival",
		"b badge",
		"B End badge",
		"c clear",
		"C closure",
		"D departure",
		"e enter",
		"f force price",
		"F function",
		"i invoice",
		"l cancel last",
		"m suspend",
		"p pay",         // (code x amount)",
		"P pay by cash", // (amount)",
		"q quit",
		"r return item",
		"R return ticket",
		"s sub-total",
		"S sync",
		"t total",
		"T transaction",
		"y Activepese",
		"Y desactivepese",
		"z etat",
	}

	// POS display
	p := widgets.NewParagraph()
	p.Title = "POS display"
	p.Text = strings.Join(ds.posDisplay[:], "\n")
	p.SetRect(0, 0, 24, 4)
	ui.Render(p)

	// Customer display
	p = widgets.NewParagraph()
	p.Title = "Customer display"
	p.Text = strings.Join(ds.customerDisplay[:], "\n")
	p.SetRect(34, 0, 58, 4)
	ui.Render(p)

	// Input
	updateInput()

	//Help
	p = widgets.NewParagraph()
	p.Title = "Help"
	p.Text = strings.Join(help[:], "\n")
	p.SetRect(60, h-len(help)-5, 81, h-3)
	ui.Render(p)

	// Receipt
	l := widgets.NewList()
	l.Title = "Receipt"
	l.Rows = ds.receipt
	l.TextStyle = ui.NewStyle(ui.ColorYellow)
	l.WrapText = false
	l.SetRect(0, 7, LINE_SIZE+2, 4+LINE_COUNT-1)
	// mutex.Lock()
	// defer mutex.Unlock()
	ui.Render(l)
}

// Engine commands
func cmd(data map[string]any) error {
	u := map[string]any{
		"seq":     10,
		"ean":     "",
		"qte":     "",
		"_ac":     "POS",
		"tiroir":  2,
		"dt":      "2021-05-14",
		"idens":   1,
		"typcode": 255,
		"typscan": 3,
		"scan":    0,
		"idm":     101,
		"fnc":     "",
		"idh":     9,
		"trs":     17,
		"idc":     1,
	}

	for key, value := range data { // Order not specified
		u[key] = value
	}

	b, err := json.Marshal(u)
	if err != nil {
		return err //panic(err)
	}

	if apiConnected {
		err = apiConnection.WriteMessage(websocket.TextMessage, b)
	} else {
		err = errors.New("no connection")
	}

	return err
}
func clearcmd() error {
	if posInput != "" {
		posInput = ""
		updateInput()
	}
	data := map[string]any{"fnc": 69}
	return cmd(data)
}
func badgecmd() error {
	data := map[string]any{"_key": "badge", "_ac": "orkpos", "_op": "sco", "tiroir": 2, "badge": posInput}

	err := cmd(data)
	if err == nil {
		posInput = ""
		updateInput()
	}
	return err
}

func activepeseScocmd(active bool) error {
	if ENGINE_MODE == 1 {
		data := map[string]any{"_key": "set", "_ac": "orkpos", "_op": "sco", "tiroir": 2}
		if active {
			data["activepese"] = 1
		} else {
			data["activepese"] = 0
		}
		err := raw(data)
		return err
	}
	return nil
}

func etatScocmd() error {
	if ENGINE_MODE == 1 {
		data := map[string]any{"_key": "etat", "_ac": "orkpos", "_op": "sco", "tiroir": 2}
		err := raw(data)
		return err
	}
	return nil
}

func EndBadgecmd() error {

	if ENGINE_MODE == 1 {
		data := map[string]any{"_key": "set", "activepese": 1, "_ac": "orkpos", "_op": "sco", "tiroir": 2}
		err := raw(data)
		return err
	}
	return nil
}

func raw(data map[string]any) error {

	b, err := json.Marshal(data)
	if err != nil {
		return err //panic(err)
	}

	if apiConnected {
		err = apiConnection.WriteMessage(websocket.TextMessage, b)
	} else {
		err = errors.New("no connection")
	}

	return err
}

func entercmd() error {

	data := map[string]any{"fnc": 68}
	if posInput != "" {
		s := strings.Split(posInput, "x")
		if len(s) == 2 {
			data["ean"] = s[1]
			data["qte"] = s[0]
		} else {
			data["ean"] = s[0]
		}
	}

	err := cmd(data)
	if err == nil {
		posInput = ""
	}
	return err
}
func synccmd() error {
	u := map[string]any{
		"_ac":   "orkpos",
		"_op":   "sync",
		"_key":  "sync",
		"appli": "tuipos",
	}

	b, err := json.Marshal(u)
	if err != nil {
		return err //panic(err)
	}

	if apiConnected {
		err = apiConnection.WriteMessage(websocket.TextMessage, b)
	} else {
		err = errors.New("no connection")
	}

	return err

}
func totalcmd() error {
	data := map[string]any{"fnc": 76}
	return cmd(data)
}

func trscmd() error {
	if posInput != "" {
		if _, err := strconv.Atoi(posInput); err == nil {
			data := map[string]any{"fnc": 178, "ean": posInput}
			posInput = ""
			updateInput()
			return cmd(data)
		} else {
			posInput = ""
			updateInput()
			return nil
		}
	}
	return nil
}

func arrivecmd() error {
	data := map[string]any{"fnc": 16}
	return cmd(data)
}
func closurecmd() error {
	data := map[string]any{"fnc": 18}
	return cmd(data)
}
func abortcmd() error {
	data := map[string]any{"fnc": 67}
	return cmd(data)
}
func suspendcmd() error {
	data := map[string]any{"fnc": 27}
	return cmd(data)
}
func departcmd() error {
	data := map[string]any{"fnc": 17}
	return cmd(data)
}
func forcepricecmd() error {
	data := map[string]any{"fnc": 84}
	return cmd(data)
}
func returnitemcmd() error {
	data := map[string]any{"fnc": 33}
	return cmd(data)
}
func returntktcmd() error {
	data := map[string]any{"fnc": 24}
	return cmd(data)
}
func paymentcmd() error {
	data := map[string]any{"trs": 24}
	if posInput != "" {
		s := strings.Split(posInput, "x")
		if len(s) == 2 {
			data["ean"] = s[1]
			data["fnc"] = s[0]
		} else {
			data["fnc"] = s[0]
		}
	} else {
		data["fnc"] = 1
	}

	err := cmd(data)
	if err == nil {
		posInput = ""
	}
	updateInput()
	return err
}
func cashcmd() error {
	data := map[string]any{"trs": 24}
	if posInput != "" {
		s := strings.Split(posInput, "x")
		if len(s) == 2 {
			data["ean"] = s[1]
			data["fnc"] = s[0]
		} else {
			data["ean"] = s[0]
			data["fnc"] = 1
		}
	} else {
		data["fnc"] = 1
	}

	err := cmd(data)
	if err == nil {
		posInput = ""
	}
	updateInput()
	return err
}
func functioncmd() error {
	if posInput != "" {
		if fnc, err := strconv.Atoi(posInput); err == nil {
			posInput = ""
			updateInput()
			data := map[string]any{"fnc": fnc}
			return cmd(data)
		} else {
			posInput = ""
			updateInput()
			return nil
		}
	}
	return nil
}
func invoicecmd() error {
	data := map[string]any{"fnc": 114}
	return cmd(data)
}
func cancelcmd() error {
	data := map[string]any{"fnc": 66}
	return cmd(data)
}
func subtotalcmd() error {
	data := map[string]any{"fnc": 75}
	return cmd(data)
}
func input(s string) {
	posInput = posInput + s
	updateInput()
}
func backspacecmd() {
	if posInput != "" {
		posInput = posInput[:len(posInput)-1]
	}
	updateInput()
}
func connectApi(terminal chan<- string) *websocket.Conn {

	apiConnected = false

	u := url.URL{Scheme: "ws", Host: *host + ":" + *port, Path: "/ws"}
	terminal <- "connecting to " + u.String()
	terminal_output_var = "connecting to " + u.String()

	for {
		var err error
		apiConnection, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
		//ac, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			apiConnected = false
			terminal <- "Error: " + err.Error()
			terminal_output_var = "Error: " + err.Error()
			time.Sleep(3 * time.Second)
			continue
		}
		//apiConnection = ac
		break
	}

	apiConnected = true

	//time.Sleep(3*time.Second)
	synccmd()
	terminal <- "connected to " + u.String()
	terminal_output_var = "connected to " + u.String()

	return apiConnection
}

func main() {
	flag.Parse()
	log.SetFlags(0)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	ds := DataStore{
		posDisplay:      []string{"", ""},
		customerDisplay: []string{"", ""},
	} // store the current state of the application

	terminal_output := make(chan string, 1)
	defer close(terminal_output)

	// Info dév
	clock := make(chan string, 1)
	defer close(clock)

	go func(t <-chan string) {
		var m string
		for {
			select {
			case m = <-t:
				p := widgets.NewParagraph()
				p.Title = "Status"
				suite := "\n"
				if ENGINE_MODE == 0 {
					suite += "POS"
				} else {
					suite += "SCO"
				}
				p.Text = m + suite
				p.SetRect(60, 0, 81, 4)
				// mutex.Lock()
				// defer mutex.Unlock()
				ui.Render(p)
			}
		}
	}(clock)
	clock <- time.Now().String()[:19]

	// Info dév

	go func(t <-chan string) {
		var m string

		for {
			select {
			case m = <-t:
				//Terminal output
				_, h := tm.Size()
				h = max(h, 12+len(lines))
				LINE_COUNT := h - 4 - 2
				p := widgets.NewParagraph()
				p.Title = "Terminal output"
				p.Text = m
				p.SetRect(0, 4+LINE_COUNT-1, 81, 4+LINE_COUNT+2)
				ui.Render(p)
			}
		}
	}(terminal_output)
	terminal_output <- ""

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	decode(&ds, "", true) // initial load

	apiConnection = connectApi(terminal_output)
	defer apiConnection.Close()

	msg := make(chan string, 1)
	done := make(chan bool, 1)

	go func(msg chan<- string) {
		defer close(done)
		for {
			_, message, err := apiConnection.ReadMessage()
			if err != nil {
				terminal_output <- err.Error()
				time.Sleep(1 * time.Second)
				apiConnection = connectApi(terminal_output)
				defer apiConnection.Close()
				//return
			} else {
				msg <- string(message)
			}
		}
	}(msg)

	var m string
	uiEvents := ui.PollEvents()

	for {
		select {
		case m = <-msg:
			decode(&ds, m, false)
		case <-done:
		case e := <-uiEvents:
			switch e.ID {
			case "c":
				clearcmd()
			case "<Enter>":
				entercmd()
			case "<Backspace>":
				backspacecmd()
			case "t":
				totalcmd()
			case "b":
				badgecmd()
			case "A":
				arrivecmd()
			case "B":
				EndBadgecmd()

			case "<Resize>":
				ui.Clear()
				tm.Sync() // flicker
				clock <- time.Now().String()[:19]
				terminal_output <- terminal_output_var
				go decode(&ds, m, true)
			case "C":
				closurecmd()
			case "D":
				departcmd()
			case "p":
				paymentcmd()
			case "P":
				cashcmd()
			case "<Escape>", "<C-c>":
				exec.Command("reset").Output()
				return
			case "S":
				synccmd()
			case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
				input(e.ID)
			case "x":
				if posInput != "" {
					if !strings.Contains(posInput, "x") {
						go input(e.ID)
					}
				}
			case "f":
				forcepricecmd()
			case "F":
				functioncmd()
			case "i":
				invoicecmd()
			case "l":
				cancelcmd()
			case "r":
				returnitemcmd()
			case "R":
				returntktcmd()
			case "s":
				subtotalcmd()
			case "T":
				trscmd()
			case "a":
				abortcmd()
			case "m":
				suspendcmd()
			case ".":
				if !(strings.Contains(posInput, "x") || strings.Contains(posInput, ".")) {
					input(e.ID)
				}
			case "y":
				activepeseScocmd(true)
			case "Y":
				activepeseScocmd(false)
			case "z":
				etatScocmd()
			}

		case <-ticker.C:
			clock <- time.Now().String()[:19]
		}
	}
}
