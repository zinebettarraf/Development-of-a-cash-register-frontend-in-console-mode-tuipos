package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type M map[string]interface{}

var (
	apiConnection *websocket.Conn
	apiConnected  = false
)

func connectApi(port, host string) *websocket.Conn {

	apiConnected = false

	log.Println("Connecting to Server", host+":"+port)

	u := url.URL{Scheme: "ws", Host: host + ":" + port, Path: "/ws"}

	for {
		ac, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			apiConnected = false
			time.Sleep(3 * time.Second)
			continue
		}
		apiConnected = true
		apiConnection = ac
		log.Println("Connected to Server", host+":"+port)

		break
	}
	return apiConnection
}

func main() {

	port := "3333"
	host := "localhost"
	filePath := ""

	//Read Flags and test file
	for _, arg := range os.Args[1:] {
		if strings.Contains(arg, "=") {
			splitted := strings.Split(arg, "=")
			if strings.Contains(splitted[0], "host") {
				host = splitted[1]
			} else if strings.Contains(splitted[0], "port") {
				port = splitted[1]
			}

		} else {
			filePath = arg
		}
	}

	readFile, err := os.Open(filePath)

	if err != nil {
		panic(err)
	}
	fileScanner := bufio.NewScanner(readFile)
	fileScanner.Split(bufio.ScanLines)
	var fileLines []string

	for fileScanner.Scan() {
		line := fileScanner.Text()
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "{") {
			line = strings.TrimSpace(line)
			line = strings.Trim(line, ",")
			line = strings.Trim(line, ";")
			line = strings.TrimSpace(line)
			fileLines = append(fileLines, line)
		}
	}

	readFile.Close()

	c := connectApi(port, host)
	defer c.Close()

	u := map[string]interface{}{"version": "5.6.109.342", "seq": "0", "ean": "", "appli": "orkpos5", "apli": "orkpos5", "qte": "", "_ac": "orkpos", "idens": "0", "_op": "sync", "_key": "sync", "idt": "0", "tiroir": 2, "idm": "0", "fnc": "0", "trs": "17", "idc": "0"}
	msg, err := json.Marshal(u)
	if err != nil {
		panic(err)
	}
	err = c.WriteMessage(websocket.TextMessage, msg)
	_, message, err := c.ReadMessage()
	if err != nil {
		panic(err)
	}
	s := string(message)
	log.Println(s[:10])

	u = map[string]interface{}{"seq": 38, "ean": "", "qte": "", "_ac": "POS", "tiroir": 2, "dt": "2020-07-14", "idens": 1, "typcode": 255, "typscan": 3, "scan": 0, "idm": 170, "fnc": "68", "idh": 99, "trs": "17", "idc": 3}
	msg, err = json.Marshal(u)
	if err != nil {

		panic(err)
	}
	err = c.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		panic(err)
	}

	i := 0
	idens, idm, idc, idt, dt := 0, 0, 0, 0, ""
	posid := ""
	seqbon := ""
	convid := make(map[string]interface{})
	updates_etats := make(map[string]interface{})
	posid_interface := make(map[string]interface{})
	beforeend := make(map[string]interface{})
	endend := make(map[string]interface{})

	skip_exchanges := []string{"orkpos", "orkidee", "end"}

	for _, line := range fileLines {

		i++
		log.Println(line)

		if strings.HasPrefix(line, "# exit") {
			log.Println("exit requested !!!")
			return
		}
		if strings.HasPrefix(line, "# sleep") {
			s, _ := strconv.ParseFloat(strings.Split(line, " ")[2], 64)
			time.Sleep(time.Duration(s) * time.Second)
			continue
		}
		if strings.HasPrefix(line, "# 20") {
			continue
		}
		if strings.HasPrefix(line, "# sql") {

			build_cmd := strings.Join(strings.Split(line, " ")[2:], " ")
			build_cmd = strings.ReplaceAll(build_cmd, "\"", "\\\"")
			cmd := fmt.Sprintf("echo \"%s\" | /home/orka/Support/bin/db", build_cmd)
			log.Println(cmd)
			_, err := exec.Command("pwd").Output()
			if err != nil {
				log.Fatal(err)
			}
			continue
		}
		if strings.HasPrefix(line, "# ") {
			log.Println(line[2:])
			continue
		}

		req := make(map[string]interface{})
		err := json.Unmarshal([]byte(line), &req)
		if err != nil {
			panic(err)
		}

		ac := fmt.Sprintf("%v", req["_ac"])
		op := fmt.Sprintf("%v", req["_op"])
		key := fmt.Sprintf("%v", req["_key"])

		if contains(skip_exchanges, ac) &&
			contains(skip_exchanges, op) &&
			contains(skip_exchanges, key) {
			log.Println("skip (ac=%s,op=%s,key=%s)", ac, op, key)
			continue
		}

		if key == "update_etat" {
			if isMapContains(updates_etats, op) {
				req["posid"] = posid_interface["posid"]
				delete(updates_etats, op)
			} else {
				updates_etats[op] = 1
				posid_interface = req
				continue
			}
		} else if key == "beforeend" {
			beforeend = req
		} else if key == "endend" {
			endend = req
		}

		conversion := false

		if true {

			if isMapContains(req, "posid") {

				for posid == "" {
					log.Println("<<<<wait for posid>>>>")
					_, message, err := c.ReadMessage()
					if err != nil {
						panic(err)
					}

					jrsp := make(map[string]interface{})
					err = json.Unmarshal(message, &jrsp)
					if err != nil {
						panic(err)
					}
					var lrsp []map[string]interface{}

					if isMapContains(jrsp, "seqb") && isMapContains(jrsp, "seqe") {
						fmt.Println(jrsp["seqb"], jrsp["seqe"])

						seqb := int(jrsp["seqb"].(float64))
						seqe := int(jrsp["seqe"].(float64))
						log.Println("@DEBUG@ req: seqb:", seqb, " seqe:", seqe)
						for iseq := seqb; iseq < seqe+1; iseq++ {
							log.Println("@@@@@@ boucle @@@@@ ", iseq, " trouvé ? ", isMapContains(jrsp, strconv.Itoa(iseq)))
							if isMapContains(jrsp, strconv.Itoa(iseq)) {
								lrsp = append(lrsp, jrsp[strconv.Itoa(iseq)].(map[string]interface{}))
							}
						}

					} else {
						lrsp = append(lrsp, jrsp)
					}

					for _, rsp := range lrsp {
						if rsp["_op"] == "conversion" {
							conversion = true
							log.Println(rsp)
							for _, rec := range rsp["data"].([]M) {
								krec := fmt.Sprintf("%v,%v,%v", rec["idcmp"], rec["idact"], rec["idcpt"])
								convid[krec] = rec["id"]
							}

						}
						if isMapContains(rsp, "dt") {
							dt = rsp["dt"].(string)
						}
						if isMapContains(rsp, "idens") {
							idens = int(rsp["idens"].(float64))
						}
						if isMapContains(rsp, "idm") {
							idm = int(rsp["idm"].(float64))
						}
						if isMapContains(rsp, "idc") {
							idc = int(rsp["idc"].(float64))
						}
						if isMapContains(rsp, "idt") {
							idt = int(rsp["idt"].(float64))
						}
						if isMapContains(rsp, "seqbon") {
							seqbon = rsp["seqbon"].(string)
						}
						if isMapContains(rsp, "posid") {
							posid = rsp["posid"].(string)
							log.Println("<<<<new posid: ", posid)
							break
						}

					}
				}
				req["posid"] = posid

				if isMapContains(req, "seqbon") {
					req["seqbon"] = seqbon
				}
				if isMapContains(req, "dt") && dt != "" {
					req["dt"] = dt
				}
				if isMapContains(req, "idens") && idens != 0 {
					req["idens"] = idens
				}
				if isMapContains(req, "idm") && idm != 0 {
					req["idm"] = idm
				}
				if isMapContains(req, "idc") && idc != 0 {
					req["idc"] = idc
				}
				if isMapContains(req, "idt") && idt != 0 {
					req["idt"] = idt
				}

			}
		}

		if !conversion && req["_op"] == "conversion" {

			fmt.Println("fedffefe")
			kreq := fmt.Sprintf("%v,%v,%v", req["idcmp"], req["idact"], req["idcpt"])
			for !isMapContains(convid, kreq) {
				log.Println("<<<<wait for conversion id ", kreq, " >>>>")
				_, message, err := c.ReadMessage()
				if err != nil {
					panic(err)
				}
				jrsp := make(map[string]interface{})
				err = json.Unmarshal(message, &jrsp)
				if err != nil {
					panic(err)
				}

				var lrsp []map[string]interface{}
				if isMapContains(jrsp, "seqb") {
					if isMapContains(jrsp, "seqe") {
						seqb := int(jrsp["seqb"].(float64))
						seqe := int(jrsp["seqe"].(float64))
						for iseq := seqb; iseq < seqe+1; iseq++ {
							if isMapContains(jrsp, strconv.Itoa(iseq)) {
								lrsp = append(lrsp, jrsp[strconv.Itoa(iseq)].(map[string]interface{}))
							}
						}
					}
				} else {
					lrsp = append(lrsp, jrsp)
				}

				for _, rsp := range lrsp {
					if rsp["_op"] == "conversion" {
						for _, rec := range rsp["data"].([]interface{}) {
							rec0 := rec.(map[string]any)
							krec := fmt.Sprintf("%v,%v,%v", rec0["idcmp"], rec0["idact"], rec0["idcpt"])
							log.Println("<<<<RECEIVED conversion id ", krec, " => ", rec0["id"], " >>>>>")
							convid[krec] = rec0["id"]
						}
					}
				}
			}
			req["id"] = convid[kreq]
		}

		if req["_op"] == "conversion" {
			log.Println(convid)
			kreq := fmt.Sprintf("%v,%v,%v", req["idcmp"], req["idact"], req["idcpt"])
			log.Println("%v", kreq)

		}

		msg, err = json.Marshal(req)
		if err != nil {
			panic(err)
		}

		b, err := json.Marshal(req)
		if err != nil {
			panic(err)
		}
		err = c.WriteMessage(websocket.TextMessage, b)
		if err != nil {
			panic(err)
		}

		if isMapContains(req, "activepese") {
			if req["_key"] == "set" && req["_op"] == "sco" && req["_ac"] == "orkpos" {
				continue
			}
		}

		log.Println("i:", i, " < len(lines):",
			len(fileLines))

		if i < len(fileLines) {

			_, message, err := c.ReadMessage()
			if err != nil {
				panic(err)
			}

			jrsp := make(map[string]interface{})
			err = json.Unmarshal(message, &jrsp)
			if err != nil {
				panic(err)
			}

			var lrsp []map[string]interface{}
			if isMapContains(jrsp, "seqb") {
				if isMapContains(jrsp, "seqe") {
					seqb := int(jrsp["seqb"].(float64))
					seqe := int(jrsp["seqe"].(float64))
					for iseq := seqb; iseq < seqe+1; iseq++ {
						if isMapContains(jrsp, strconv.Itoa(iseq)) {
							lrsp = append(lrsp, jrsp[strconv.Itoa(iseq)].(map[string]interface{}))
						}
					}
				}
			} else {
				lrsp = append(lrsp, jrsp)
			}

			for _, rsp := range lrsp {

				if rsp["_op"] == "conversion" {
					for _, rec := range rsp["data"].([]interface{}) {
						rec0 := rec.(map[string]any)
						krec := fmt.Sprintf("%v,%v,%v", rec0["idcmp"], rec0["idact"], rec0["idcpt"])
						log.Println("<<<<RECEIVED conversion id ", rec0["id"], ">>>>", rec0["id"])
						convid[krec] = rec0["id"]
					}
				}
				if isMapContains(rsp, "dt") {
					dt = rsp["dt"].(string)
				}
				if isMapContains(rsp, "idens") {
					idens = int(rsp["idens"].(float64))
				}
				if isMapContains(rsp, "idm") {
					idm = int(rsp["idm"].(float64))
				}
				if isMapContains(rsp, "idc") {
					idc = int(rsp["idc"].(float64))
				}
				if isMapContains(rsp, "idt") {
					idt = int(rsp["idt"].(float64))
				}
				op = ""
				key = ""

				if isMapContains(rsp, "_op") {
					op = rsp["_op"].(string)
				}
				if isMapContains(rsp, "_key") {
					op = rsp["_key"].(string)
				}

				if key == "update_etat" {
					if isMapContains(updates_etats, op) {
						req = posid_interface
						req["posid"] = rsp["posid"]

						if isMapContains(req, "dt") && dt != "" {
							req["dt"] = dt
						}
						if isMapContains(req, "idens") && idens != 0 {
							req["idens"] = idens
						}
						if isMapContains(req, "idm") && idm != 0 {
							req["idm"] = idm
						}
						if isMapContains(req, "idc") && idc != 0 {
							req["idc"] = idc
						}
						if isMapContains(req, "idt") && idt != 0 {
							req["idt"] = idt
						}
						b, err := json.Marshal(req)
						if err != nil {
							panic(err)
						}
						err = c.WriteMessage(websocket.TextMessage, b)
						if err != nil {
							panic(err)
						}

						delete(updates_etats, op)

					} else {
						updates_etats[op] = req
					}
					break
				} else if key == "beforeend" {
					if len(beforeend) > 0 {
						req0 := req
						req := beforeend
						req["posid"] = rsp["posid"]

						if isMapContains(req, "dt") && dt != "" {
							req["dt"] = dt
						}
						if isMapContains(req, "idens") && idens != 0 {
							req["idens"] = idens
						}
						if isMapContains(req, "idm") && idm != 0 {
							req["idm"] = idm
						}
						if isMapContains(req, "idc") && idc != 0 {
							req["idc"] = idc
						}
						if isMapContains(req, "idt") && idt != 0 {
							req["idt"] = idt
						}
						b, err := json.Marshal(req)
						if err != nil {
							panic(err)
						}
						err = c.WriteMessage(websocket.TextMessage, b)
						if err != nil {
							panic(err)
						}

						beforeend = make(map[string]interface{})
						b, err = json.Marshal(req0)
						if err != nil {
							panic(err)
						}
						err = c.WriteMessage(websocket.TextMessage, b)
						if err != nil {
							panic(err)
						}

						break
					}
				} else if key == "endend" {
					if len(endend) > 0 {
						req0 := req
						req := endend
						req["posid"] = rsp["posid"]

						if isMapContains(req, "dt") && dt != "" {
							req["dt"] = dt
						}
						if isMapContains(req, "idens") && idens != 0 {
							req["idens"] = idens
						}
						if isMapContains(req, "idm") && idm != 0 {
							req["idm"] = idm
						}
						if isMapContains(req, "idc") && idc != 0 {
							req["idc"] = idc
						}
						if isMapContains(req, "idt") && idt != 0 {
							req["idt"] = idt
						}
						b, err := json.Marshal(req)
						if err != nil {
							panic(err)
						}
						err = c.WriteMessage(websocket.TextMessage, b)
						if err != nil {
							panic(err)
						}

						endend = make(map[string]interface{})
						b, err = json.Marshal(req0)
						if err != nil {
							panic(err)
						}
						err = c.WriteMessage(websocket.TextMessage, b)
						if err != nil {
							panic(err)
						}

						break
					}
				} else if isMapContains(rsp, "posid") {
					posid = rsp["posid"].(string)
					if isMapContains(rsp, "seqbon") {
						seqbon = rsp["seqbon"].(string)
					}
					log.Println("<<<<new posid while: ", posid)
					break
				} else {
					posid = ""
					seqbon = ""
				}

			}
		}
	}

}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

func isMapContains(dict map[string]interface{}, champ string) bool {
	if _, ok := dict[champ]; ok {
		return true
	}
	return false
}
