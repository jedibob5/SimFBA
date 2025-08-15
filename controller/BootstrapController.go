package controller

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/CalebRose/SimFBA/managers"
	"github.com/gorilla/mux"
	"github.com/mailru/easyjson"
)

func BootstrapTeamData(w http.ResponseWriter, r *http.Request) {
	data := managers.GetTeamsBootstrap()
	w.Header().Set("Content-Type", "application/json")
	teamData, err := easyjson.Marshal(data)
	if err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
	w.Write(teamData)
}

func FirstBootstrapFootballData(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	vars := mux.Vars(r)
	collegeID := vars["collegeID"]
	proID := vars["proID"]
	data := managers.GetFirstBootstrapData(collegeID, proID)
	bootstrapData, err := easyjson.Marshal(data)
	if err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
	w.Write(bootstrapData)
}

func SecondBootstrapFootballData(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	data := managers.GetSecondBootstrapData()
	bootstrapData, err := easyjson.Marshal(data)
	if err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
	w.Write(bootstrapData)
}

func ThirdBootstrapFootballData(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	data := managers.GetThirdBootstrapData()
	bootstrapData, err := easyjson.Marshal(data)
	if err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
	w.Write(bootstrapData)
}

func GetCollegeHistoryProfile(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	data := managers.GetCollegeTeamProfilePageData()
	json.NewEncoder(w).Encode(data)
}
