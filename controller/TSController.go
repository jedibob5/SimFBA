package controller

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/CalebRose/SimFBA/managers"
	"github.com/CalebRose/SimFBA/models"
	"github.com/CalebRose/SimFBA/structs"
	"github.com/tkrajina/typescriptify-golang-structs/typescriptify"
)

type Fire struct {
	Key           string
	Domain        string
	DBUrl         string
	ProjectID     string
	Bucket        string
	SenderID      string
	AppID         string
	MeasurementID string
	User          string
	UserKey       string
}

func FireItUp(w http.ResponseWriter, r *http.Request) {

	fire := Fire{
		Key:           os.Getenv("VITE_FIREBASE_API_KEY"),
		Domain:        os.Getenv("VITE_FIREBASE_AUTH_DOMAIN"),
		DBUrl:         os.Getenv("VITE_FIREBASE_DATABASE_URL"),
		ProjectID:     os.Getenv("VITE_FIREBASE_PROJECT_ID"),
		Bucket:        os.Getenv("VITE_FIREBASE_STORAGE_BUCKET"),
		SenderID:      os.Getenv("VITE_FIREBASE_MESSAGING_SENDER_ID"),
		AppID:         os.Getenv("VITE_FIREBASE_APP_ID"),
		MeasurementID: os.Getenv("VITE_FIREBASE_MEASUREMENT_ID"),
		User:          os.Getenv("VITE_SIMFBA_KEY"),
		UserKey:       os.Getenv("VITE_SIMFBA_USER"),
	}
	json.NewEncoder(w).Encode(fire)
}

func CreateTSModelsFile(w http.ResponseWriter, r *http.Request) {
	converter := typescriptify.New().
		Add(managers.BootstrapDataCommon{}).
		Add(managers.BootstrapDataOne{}).
		Add(managers.BootstrapDataTwo{}).
		Add(managers.BootstrapDataThree{}).
		Add(managers.CollegeTeamProfileData{}).
		Add(structs.SearchStatsResponse{}).
		Add(structs.FaceDataResponse{}).
		Add(structs.TeamRequestsResponse{}).
		Add(structs.BasePlayer{}).
		Add(structs.CollegePlayer{}).
		Add(structs.NFLPlayer{}).
		Add(structs.BaseTeam{}).
		Add(structs.CollegeTeam{}).
		Add(structs.NFLTeam{}).
		Add(structs.Stadium{}).
		Add(structs.CollegeStandings{}).
		Add(structs.NFLStandings{}).
		Add(structs.Recruit{}).
		Add(structs.RecruitPlayerProfile{}).
		Add(structs.RecruitingTeamProfile{}).
		Add(structs.Croot{}).
		Add(structs.LeadingTeams{}).
		Add(structs.CreateRecruitProfileDto{}).
		Add(structs.UpdateRecruitPointsDto{}).
		Add(structs.CrootProfile{}).
		Add(structs.SimTeamBoardResponse{}).
		Add(structs.UpdateRecruitingBoardDTO{}).
		Add(structs.RecruitPointAllocation{}).
		Add(structs.ProfileAffinity{}).
		Add(structs.RedshirtDTO{}).
		Add(structs.CFBRosterPageResponse{}).
		Add(structs.CollegePromise{}).
		Add(structs.TransferPortalProfile{}).
		Add(structs.TransferPortalProfileResponse{}).
		Add(structs.TransferPortalResponse{}).
		Add(structs.TransferPortalBoardDto{}).
		Add(structs.UpdateTransferPortalBoard{}).
		Add(structs.CollegeGame{}).
		Add(structs.NFLCapsheet{}).
		Add(structs.NFLContract{}).
		Add(structs.FreeAgencyOffer{}).
		Add(structs.FreeAgencyOfferDTO{}).
		Add(structs.NFLWaiverOffDTO{}).
		Add(structs.NFLExtensionOffer{}).
		Add(structs.CollegePollSubmission{}).
		Add(structs.CollegePollOfficial{}).
		Add(structs.PollDataResponse{}).
		Add(structs.NFLWaiverOffer{}).
		Add(structs.NFLDraftPick{}).
		Add(models.NFLDraftee{}).
		Add(models.NFLDraftPageResponse{}).
		Add(models.NFLWarRoom{}).
		Add(models.ScoutingProfile{}).
		Add(models.ScoutingProfileDTO{}).
		Add(models.ScoutingDataResponse{}).
		Add(models.RevealAttributeDTO{}).
		Add(models.ExportDraftPicksDTO{}).
		Add(structs.CollegePlayerResponse{}).
		Add(structs.NFLPlayerResponse{}).
		Add(structs.CollegePlayerCSV{}).
		Add(structs.CollegePlayerSeasonStats{}).
		Add(structs.CollegePlayerStats{}).
		Add(structs.NFLPlayerStats{}).
		Add(structs.NFLPlayerSeasonStats{}).
		Add(structs.CollegeTeamStats{}).
		Add(structs.CollegeTeamSeasonStats{}).
		Add(structs.NFLTeamStats{}).
		Add(structs.NFLTeamSeasonStats{}).
		Add(structs.CollegeTeamDepthChart{}).
		Add(structs.CollegeDepthChartPosition{}).
		Add(structs.CollegeGameplan{}).
		Add(structs.NFLDepthChart{}).
		Add(structs.NFLDepthChartPosition{}).
		Add(structs.NFLGameplan{}).
		Add(structs.HistoricCollegePlayer{}).
		Add(structs.NFLRetiredPlayer{}).
		Add(structs.NFLRequest{}).
		Add(structs.TeamRequest{}).
		Add(structs.NFLTradeProposal{}).
		Add(structs.NFLTradeProposalDTO{}).
		Add(structs.NFLTradeOption{}).
		Add(structs.NFLTradeOptionObj{}).
		Add(structs.NFLTeamProposals{}).
		Add(structs.NFLTradePreferences{}).
		Add(structs.NFLTradePreferencesDTO{}).
		Add(structs.NFLUser{}).
		Add(structs.CollegeCoach{}).
		Add(structs.PlayByPlayResponse{}).
		Add(structs.GameResultsResponse{}).
		Add(structs.GameResultsPlayer{}).
		Add(models.TeamRecordResponse{}).
		Add(models.TopPlayer{}).
		Add(structs.InboxResponse{}).
		Add(structs.NewsLog{}).
		Add(structs.Notification{}).
		Add(structs.CollusionDto{}).
		Add(structs.Timestamp{})
	err := converter.ConvertToFile("ts/footballModels.ts")
	if err != nil {
		panic(err.Error())
	}
	json.NewEncoder(w).Encode("Models ran!")
}
