package playground

import (
	"NetManager/env"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"net/http"
)

type SyncPacket struct {
	EntryList []env.TableEntry `json:"entry_list"`
}

func AskSync(ip string, port string, entries [][]string) error {
	RequestUrl := fmt.Sprintf("http://%s:%s/sync", ip, port)
	req := SyncPacket{
		EntryList: entriesToList(entries),
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	response, err := http.Post(RequestUrl, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	respBody, _ := ioutil.ReadAll(response.Body)
	result := SyncPacket{}
	err = json.Unmarshal(respBody, &result)
	if err != nil {
		return err
	}
	for _, entry := range result.EntryList {
		AddRoute(entry)
	}
	return nil
}

func HandleHttpSyncRequests(port string) {
	netRouter := mux.NewRouter().StrictSlash(true)
	netRouter.HandleFunc("/sync", handleSync).Methods("POST")
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), netRouter))
}

func handleSync(writer http.ResponseWriter, request *http.Request) {
	reqBody, err := ioutil.ReadAll(request.Body)
	if err != nil {
		writer.WriteHeader(500)
		return
	}
	syncPacket := SyncPacket{}
	err = json.Unmarshal(reqBody, &syncPacket)
	if err != nil {
		writer.WriteHeader(500)
		return
	}
	for _, entry := range syncPacket.EntryList {
		AddRoute(entry)
	}
	syncPacket.EntryList = entriesToList(Entries)
	body, err := json.Marshal(syncPacket)
	if err != nil {
		writer.WriteHeader(500)
		return
	}
	_, err = writer.Write(body)
	if err != nil {
		writer.WriteHeader(500)
		return
	}

}

func entriesToList(entries [][]string) []env.TableEntry {
	res := make([]env.TableEntry, 0)
	for _, entry := range entries {
		res = append(res, StringToEntry(entry))
	}
	return res
}
