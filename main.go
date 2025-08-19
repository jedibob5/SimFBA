package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CalebRose/SimFBA/controller"
	"github.com/CalebRose/SimFBA/dbprovider"
	"github.com/CalebRose/SimFBA/middleware"
	"github.com/CalebRose/SimFBA/structs"
	"github.com/CalebRose/SimFBA/ws"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"github.com/joho/godotenv"
	"github.com/nelkinda/health-go"
	"github.com/nelkinda/health-go/checks/sendgrid"
	"github.com/robfig/cron/v3"
	cache "github.com/victorspringer/http-cache"
	"github.com/victorspringer/http-cache/adapter/memory"
)

func InitialMigration() {
	initiate := dbprovider.GetInstance().InitDatabase()
	if !initiate {
		log.Println("Initiate pool failure... Ending application")
		os.Exit(1)
	}
}

func monitorDBForUpdates() {
	var ts structs.Timestamp
	for {
		currentTS := controller.GetUpdatedTimestamp()
		if currentTS.UpdatedAt.After(ts.UpdatedAt) {
			ts = currentTS
			err := ws.BroadcastTSUpdate(ts)
			if err != nil {
				log.Printf("Error broadcasting timestamp: %v", err)
			}
		}

		time.Sleep(60 * time.Second)
	}
}

func handleRequests() http.Handler {
	myRouter := mux.NewRouter().StrictSlash(true)

	// Handler & Middleware
	loadEnvs()
	origins := os.Getenv("ORIGIN_ALLOWED")
	originsOk := handlers.AllowedOrigins([]string{origins})
	headersOk := handlers.AllowedHeaders([]string{"Content-Type", "Authorization", "Accept", "X-Requested-With", "Access-Control-Request-Method", "Access-Control-Request-Headers", "Access-Control-Allow-Origin"})
	methodsOk := handlers.AllowedMethods([]string{"GET", "POST", "OPTIONS", "PUT", "HEAD", "DELETE"})
	apiRouter := myRouter.PathPrefix("/api").Subrouter()
	apiRouter.Use(middleware.GzipMiddleware)

	memcached, err := memory.NewAdapter(
		memory.AdapterWithAlgorithm(memory.LRU),
		memory.AdapterWithStorageCapacity(1000000000), // try 1GB max capacity at first
	)
	if err != nil {
		log.Fatal(err)
	}

	cacheClient, err := cache.NewClient(
		cache.ClientWithAdapter(memcached),
		cache.ClientWithTTL(10 * time.Minute),
		cache.ClientWithRefreshKey("opn"),
	)
	if err != nil {
		log.Fatal(err)
	}
	cachedApiRouter := myRouter.PathPrefix("/api").Subrouter()
	cachedApiRouter.Use(middleware.GzipMiddleware)
	cachedApiRouter.Use(cacheClient.Middleware)

	// Health Controls
	HealthCheck := health.New(
		health.Health{
			Version:   "1",
			ReleaseID: "0.0.7-SNAPSHOT",
		},
		sendgrid.Health(),
	)
	myRouter.HandleFunc("/health", HealthCheck.Handler).Methods("GET")

	// Admin Controls
	apiRouter.HandleFunc("/admin/generate/ts/models/", controller.CreateTSModelsFile).Methods("GET")
	// apiRouter.HandleFunc("/admin/fire/it/up/", controller.FireItUp).Methods("GET")
	apiRouter.HandleFunc("/simfba/get/timestamp/", controller.GetCurrentTimestamp).Methods("GET")
	apiRouter.HandleFunc("/simfba/sync/timestamp/", controller.SyncTimestamp).Methods("POST")
	apiRouter.HandleFunc("/simfba/sync/week/", controller.SyncWeek).Methods("GET")
	apiRouter.HandleFunc("/simfba/sync/timeslot/{timeslot}", controller.SyncTimeslot).Methods("GET")
	// apiRouter.HandleFunc("/simfba/regress/timeslot/{timeslot}", controller.RegressTimeslot).Methods("GET")
	apiRouter.HandleFunc("/simfba/sync/freeagency/round", controller.SyncFreeAgencyRound).Methods("GET")
	apiRouter.HandleFunc("/simfba/sync/recruiting/", controller.SyncRecruiting).Methods("GET")
	// apiRouter.HandleFunc("/simfba/sync/season/", controller.SyncToNextSeason).Methods("GET")
	// apiRouter.HandleFunc("/simfba/sync/missing/", controller.SyncMissingRES).Methods("GET")
	apiRouter.HandleFunc("/simfba/mass/{off}/{def}", controller.MassUpdateGameplans).Methods("GET")
	apiRouter.HandleFunc("/simfba/sync/weather/", controller.WeatherGenerator).Methods("GET")
	apiRouter.HandleFunc("/simfba/current/weather/forecast/", controller.GetWeatherForecast).Methods("GET")
	apiRouter.HandleFunc("/simfba/future/weather/forecast/", controller.GetFutureWeatherForecast).Methods("GET")
	apiRouter.HandleFunc("/news/{weekID}/{seasonID}/", controller.GetNewsLogs).Methods("GET")
	apiRouter.HandleFunc("/season/{seasonID}/weeks/{weekID}", controller.GetWeeksInSeason).Methods("GET")
	apiRouter.HandleFunc("/admin/teams/croot/sync", controller.SyncTeamRecruitingRanks).Methods("GET")
	apiRouter.HandleFunc("/admin/recruiting/class/size", controller.GetRecruitingClassSizeForTeams).Methods("GET")
	apiRouter.HandleFunc("/admin/ai/fill/boards", controller.FillAIBoards).Methods("GET")
	apiRouter.HandleFunc("/admin/ai/sync/boards", controller.SyncAIBoards).Methods("GET")
	apiRouter.HandleFunc("/admin/fix/affinities", controller.RecalibrateCrootProfiles).Methods("GET")
	apiRouter.HandleFunc("/admin/run/the/games/", controller.RunTheGames).Methods("GET")
	// apiRouter.HandleFunc("/admin/overall/progressions/next/season", controller.ProgressToNextSeason).Methods("GET")
	apiRouter.HandleFunc("/admin/overall/progressions/nfl", controller.ProgressNFL).Methods("GET")
	apiRouter.HandleFunc("/admin/trades/accept/sync/{proposalID}", controller.SyncAcceptedTrade).Methods("GET")
	apiRouter.HandleFunc("/admin/trades/veto/sync/{proposalID}", controller.VetoAcceptedTrade).Methods("GET")
	apiRouter.HandleFunc("/admin/trades/cleanup", controller.CleanUpRejectedTrades).Methods("GET")

	// Bootstrap
	cachedApiRouter.HandleFunc("/bootstrap/teams/", controller.BootstrapTeamData).Methods("GET")
	cachedApiRouter.HandleFunc("/bootstrap/one/{collegeID}/{proID}", controller.FirstBootstrapFootballData).Methods("GET")
	cachedApiRouter.HandleFunc("/bootstrap/two", controller.SecondBootstrapFootballData).Methods("GET")
	cachedApiRouter.HandleFunc("/bootstrap/three", controller.ThirdBootstrapFootballData).Methods("GET")

	// Capsheet Controls
	apiRouter.HandleFunc("/nfl/capsheet/generate", controller.GenerateCapsheets).Methods("GET")
	apiRouter.HandleFunc("/nfl/contracts/get/value", controller.CalculateContracts).Methods("GET")

	// Draft Controls
	apiRouter.HandleFunc("/nfl/draft/draftees/export/{season}", controller.ExportDrafteesToCSV).Methods("GET")
	apiRouter.HandleFunc("/nfl/draft/export/picks", controller.ExportDraftedPicks).Methods("POST")
	apiRouter.HandleFunc("/nfl/draft/page/{teamID}", controller.GetDraftPageData).Methods("GET")
	apiRouter.HandleFunc("/nfl/draft/time/change", controller.ToggleDraftTime).Methods("GET")
	apiRouter.HandleFunc("/nfl/draft/create/scoutprofile", controller.AddPlayerToScoutBoard).Methods("POST")
	apiRouter.HandleFunc("/nfl/draft/reveal/attribute", controller.RevealScoutingAttribute).Methods("POST")
	apiRouter.HandleFunc("/nfl/draft/remove/{id}", controller.RemovePlayerFromScoutBoard).Methods("GET")
	apiRouter.HandleFunc("/nfl/draft/scout/{id}", controller.GetScoutingDataByDraftee).Methods("GET")
	// apiRouter.HandleFunc("/nfl/draft/boom/bust", controller.BoomOrBust).Methods("GET")

	// Face Controls
	// apiRouter.HandleFunc("/faces/migrate", controller.MigrateFaceData).Methods("GET")
	// apiRouter.HandleFunc("/spending/count/fix", controller.FixSpendingCount).Methods("GET")

	// Free Agency Controls
	// apiRouter.HandleFunc("/nfl/extensions/sync", controller.SyncExtensions).Methods("GET")
	apiRouter.HandleFunc("/nfl/extension/create/offer", controller.CreateExtensionOffer).Methods("POST")
	apiRouter.HandleFunc("/nfl/extension/cancel/offer", controller.CancelExtensionOffer).Methods("POST")
	apiRouter.HandleFunc("/nfl/freeagency/create/offer", controller.CreateFreeAgencyOffer).Methods("POST")
	apiRouter.HandleFunc("/nfl/freeagency/cancel/offer", controller.CancelFreeAgencyOffer).Methods("POST")
	apiRouter.HandleFunc("/nfl/waiverwire/create/offer", controller.CreateWaiverWireOffer).Methods("POST")
	apiRouter.HandleFunc("/nfl/waiverwire/cancel/offer", controller.CancelWaiverWireOffer).Methods("POST")
	apiRouter.HandleFunc("/nfl/freeagency/waiver/order/set", controller.SetWaiverOrderForNFLTeams).Methods("GET")

	// Game Controls
	apiRouter.HandleFunc("/games/update/time/", controller.UpdateTimeslot).Methods("POST", "OPTIONS")
	// apiRouter.HandleFunc("/games/byeweek/fix/", controller.FixByeWeekLogic).Methods("GET")
	apiRouter.HandleFunc("/games/college/week/{weekID}/", controller.GetCollegeGamesByTimeslotWeekId).Methods("GET")
	apiRouter.HandleFunc("/games/college/timeslot/{timeSlot}/{weekID}", controller.GetCollegeGamesByTimeslotWeekId).Methods("GET")
	apiRouter.HandleFunc("/games/college/team/{teamID}/{seasonID}", controller.GetCollegeGamesByTeamIDAndSeasonID).Methods("GET")
	apiRouter.HandleFunc("/games/college/season/{seasonID}", controller.GetCollegeGamesBySeasonID).Methods("GET")
	apiRouter.HandleFunc("/games/nfl/team/{teamID}/{seasonID}", controller.GetNFLGamesByTeamIDAndSeasonID).Methods("GET")
	apiRouter.HandleFunc("/games/nfl/season/{seasonID}", controller.GetNFLGamesBySeasonID).Methods("GET")
	apiRouter.HandleFunc("/games/result/cfb/{gameID}", controller.GetCollegeGameResultsByGameID).Methods("GET")
	apiRouter.HandleFunc("/games/result/nfl/{gameID}", controller.GetNFLGameResultsByGameID).Methods("GET")
	apiRouter.HandleFunc("/games/export/results/{seasonID}/{weekID}/{nflWeekID}/{timeslot}", controller.ExportCFBGameResults).Methods("GET")

	// Gameplan Controls
	apiRouter.HandleFunc("/gameplan/college/team/{teamID}/", controller.GetTeamGameplanByTeamID).Methods("GET")
	apiRouter.HandleFunc("/gameplan/college/ai/update/", controller.DetermineAIGameplan).Methods("GET")
	apiRouter.HandleFunc("/gameplan/college/updategameplan", controller.UpdateGameplan).Methods("POST")
	apiRouter.HandleFunc("/gameplan/college/depthchart/{teamID}/", controller.GetTeamDepthchartByTeamID).Methods("GET")
	apiRouter.HandleFunc("/gameplan/college/depthchart/user/check/", controller.CheckAllUserDepthChartsForInjuredPlayers).Methods("GET")
	apiRouter.HandleFunc("/gameplan/college/depthchart/ai/update/", controller.UpdateCollegeAIDepthCharts).Methods("GET")
	apiRouter.HandleFunc("/gameplan/college/depthchart/test-ai/update/", controller.UpdateCollegeAIDepthChartsTEST).Methods("GET")
	apiRouter.HandleFunc("/gameplan/college/depthchart/positions/{depthChartID}/", controller.GetDepthChartPositionsByDepthChartID).Methods("GET")
	apiRouter.HandleFunc("/gameplan/college/updatedepthchart", controller.UpdateDepthChart).Methods("PUT")
	apiRouter.HandleFunc("/gameplan/nfl/team/{teamID}/", controller.GetNFLGameplanByTeamID).Methods("GET")
	apiRouter.HandleFunc("/gameplan/nfl/updategameplan", controller.UpdateNFLGameplan).Methods("POST")
	apiRouter.HandleFunc("/gameplan/nfl/depthchart/{teamID}/", controller.GetNFLDepthChart).Methods("GET")
	apiRouter.HandleFunc("/gameplan/nfl/updatedepthchart", controller.UpdateNFLDepthChart).Methods("POST")
	apiRouter.HandleFunc("/gameplan/nfl/depthchart/ai/update/", controller.UpdateNFLAIDepthCharts).Methods("GET")

	// Generation Controls
	// apiRouter.HandleFunc("/admin/generate/walkons", controller.GenerateWalkOns).Methods("GET")

	// History Controls
	cachedApiRouter.HandleFunc("/history/college", controller.GetCollegeHistoryProfile).Methods("GET")

	// Import Controls
	// apiRouter.HandleFunc("/admin/import/fcs/gameplans", controller.GenerateNewGameplans).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/recruit/ai", controller.ImportRecruitAICSV).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/nfl/draft", controller.Import2023DraftedPlayers).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/cfb/standings", controller.ImportCFBStandings).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/cfb/coaches", controller.GenerateCoachesForAITeams).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/cfb/games", controller.ImportCFBGames).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/cfb/games", controller.ImportCFBGames).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/cfb/rivals", controller.ImportCFBRivals).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/cfb/teams", controller.ImportCFBTeams).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/nfl/games", controller.ImportNFLGames).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/nfl/warroom", controller.GenerateDraftWarRooms).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/nfl/udfas", controller.ImportUDFAs).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/missing/recruits", controller.GetMissingRecruitingClasses).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/missing/draftees", controller.ImportMissingDraftees).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/missing/cfb/stats", controller.ImportMissingStats).Methods("GET")
	// apiRouter.HandleFunc("/admin/import/preferences", controller.ImportTradePreferences).Methods("GET")
	// apiRouter.HandleFunc("/import/custom/croots", controller.ImportCustomCroots).Methods("GET")
	// apiRouter.HandleFunc("/import/simnfl/updated/values", controller.ImportSimNFLMinimumValues).Methods("GET")
	// apiRouter.HandleFunc("/import/simfba/draft/picks", controller.ImportNFLDraftPicks).Methods("GET")
	// apiRouter.HandleFunc("/import/simfba/updated/picks", controller.UpdateDraftPicksForDraft).Methods("GET")
	// apiRouter.HandleFunc("/import/simfba/fix/contracts", controller.FixBrokenExtensions).Methods("GET")
	// apiRouter.HandleFunc("/import/simfba/import/attributes", controller.ImplementPrimeAge).Methods("GET")
	// apiRouter.HandleFunc("/import/simcfb/college/standings", controller.CreateCollegeStandings).Methods("GET")
	// apiRouter.HandleFunc("/import/simcfb/2021/stats", controller.Import2021CFBStats).Methods("GET")
	// apiRouter.HandleFunc("/import/additional/dc/positions", controller.ImportAdditionalDCPositions).Methods("GET")
	// apiRouter.HandleFunc("/fix/simcfb/nfl/dts", controller.FixCollegeDTOVRs).Methods("GET")
	// apiRouter.HandleFunc("/fix/simcfb/ath/", controller.FixATHProgressions).Methods("GET")
	// apiRouter.HandleFunc("/fix/simfba/secondary/positions/", controller.FixSecondaryPositions).Methods("GET")
	// apiRouter.HandleFunc("/assign/team/grades", controller.ImportTeamGrades).Methods("GET")
	apiRouter.HandleFunc("/run/predraft/events", controller.RunPreDraftEvents).Methods("GET")

	// News Controls
	cachedApiRouter.HandleFunc("/cfb/news/all/", controller.GetAllNewsLogsForASeason).Methods("GET")
	cachedApiRouter.HandleFunc("/nfl/news/all/", controller.GetAllNFLNewsBySeason).Methods("GET")
	cachedApiRouter.HandleFunc("/news/feed/{league}/{teamID}/", controller.GetNewsFeed).Methods("GET")

	// Notification Controls
	cachedApiRouter.HandleFunc("/fba/inbox/get/{cfbID}/{nflID}/", controller.GetFBAInbox).Methods("GET")
	cachedApiRouter.HandleFunc("/notification/toggle/{notiID}", controller.ToggleNotificationAsRead).Methods("GET")
	cachedApiRouter.HandleFunc("/notification/delete/{notiID}", controller.DeleteNotification).Methods("GET")

	// Player Controls
	cachedApiRouter.HandleFunc("/players/all/", controller.AllPlayers).Methods("GET")
	apiRouter.HandleFunc("/collegeplayers/cut/player/{PlayerID}/", controller.CutCFBPlayerFromRoster).Methods("GET")
	apiRouter.HandleFunc("/collegeplayers/heisman/", controller.GetHeismanList).Methods("GET")
	cachedApiRouter.HandleFunc("/collegeplayers/team/{teamID}/", controller.AllCollegePlayersByTeamID).Methods("GET")
	cachedApiRouter.HandleFunc("/collegeplayers/team/nors/{teamID}/", controller.AllCollegePlayersByTeamIDWithoutRedshirts).Methods("GET")
	cachedApiRouter.HandleFunc("/collegeplayers/team/export/{teamID}/", controller.ExportRosterToCSV).Methods("GET")
	apiRouter.HandleFunc("/collegeplayers/assign/redshirt/{PlayerID}", controller.ToggleRedshirtStatusForPlayer).Methods("GET", "OPTIONS")
	cachedApiRouter.HandleFunc("/nflplayers/team/{teamID}/", controller.AllNFLPlayersByTeamIDForDC).Methods("GET")
	cachedApiRouter.HandleFunc("/nflplayers/freeagency/available/{teamID}", controller.FreeAgencyAvailablePlayers).Methods("GET")
	cachedApiRouter.HandleFunc("/nflplayers/team/export/{teamID}/", controller.ExportNFLRosterToCSV).Methods("GET")
	apiRouter.HandleFunc("/nflplayers/tag/player/", controller.TagPlayer).Methods("POST")
	apiRouter.HandleFunc("/nflplayers/cut/player/{PlayerID}/", controller.CutNFLPlayerFromRoster).Methods("GET")
	apiRouter.HandleFunc("/nflplayers/place/player/squad/{PlayerID}/", controller.PlaceNFLPlayerOnPracticeSquad).Methods("GET")
	apiRouter.HandleFunc("/nflplayers/injury/reserve/player/{PlayerID}/", controller.PlaceNFLPlayerOnInjuryReserve).Methods("GET")
	apiRouter.HandleFunc("/collegeplayers/teams/export/", controller.ExportAllRostersToCSV).Methods("GET") // DO NOT USE

	// Poll Controls
	apiRouter.HandleFunc("/college/poll/create/", controller.CreatePollSubmission).Methods("POST")
	apiRouter.HandleFunc("/college/poll/sync", controller.SyncCollegePoll).Methods("GET")
	apiRouter.HandleFunc("/college/poll/official/season/{seasonID}", controller.GetOfficialPollsBySeasonID).Methods("GET")
	apiRouter.HandleFunc("/college/poll/submission/{username}", controller.GetPollSubmission).Methods("GET")

	// Rankings Controls
	// apiRouter.HandleFunc("/simfba/cfb/croots/generate/", controller.GenerateRecruits).Methods("GET")
	// apiRouter.HandleFunc("/rankings/assign/all/croots/", controller.AssignAllRecruitRanks).Methods("GET")

	// Recruiting Controls
	apiRouter.HandleFunc("/recruiting/overview/dashboard/{teamID}", controller.GetRecruitingProfileForDashboardByTeamID).Methods("GET")
	// apiRouter.HandleFunc("/recruiting/profile/recalibrate/", controller.RecalibrateCrootProfiles).Methods("GET")
	apiRouter.HandleFunc("/recruiting/profile/team/{teamID}/", controller.GetRecruitingProfileForTeamBoardByTeamID).Methods("GET")
	apiRouter.HandleFunc("/recruiting/profile/all/", controller.GetAllRecruitingProfiles).Methods("GET")
	apiRouter.HandleFunc("/recruiting/profile/only/{teamID}/", controller.GetOnlyRecruitingProfileByTeamID).Methods("GET")
	apiRouter.HandleFunc("/recruiting/save/ai/", controller.ToggleAIBehavior).Methods("POST")
	apiRouter.HandleFunc("/recruiting/addrecruit/", controller.CreateRecruitPlayerProfile).Methods("POST")
	// apiRouter.HandleFunc("/recruiting/allocaterecruitpoints/", controller.AllocateRecruitingPointsForRecruit).Methods("PUT")
	apiRouter.HandleFunc("/recruiting/toggleScholarship/", controller.SendScholarshipToRecruit).Methods("POST")
	// apiRouter.HandleFunc("/recruiting/revokescholarship/", controller.RevokeScholarshipFromRecruit).Methods("PUT")
	apiRouter.HandleFunc("/recruiting/removecrootfromboard/", controller.RemoveRecruitFromBoard).Methods("PUT")
	apiRouter.HandleFunc("/recruiting/savecrootboard/", controller.SaveRecruitingBoard).Methods("POST")

	// ReCroot Controls
	apiRouter.HandleFunc("/recruits/all/", controller.AllRecruits).Methods("GET")
	apiRouter.HandleFunc("/recruits/export/all/", controller.ExportCroots).Methods("GET")
	// apiRouter.HandleFunc("/recruits/generate/", controller.ExportCroots).Methods("GET")
	// apiRouter.HandleFunc("/recruits/juco/all/", controller.AllJUCOCollegeRecruits).Methods("GET")
	apiRouter.HandleFunc("/recruits/recruit/{recruitID}/", controller.GetCollegeRecruitByRecruitID).Methods("GET")
	apiRouter.HandleFunc("/recruits/profile/recruits/{recruitProfileID}/", controller.GetRecruitsByTeamProfileID).Methods("GET")
	apiRouter.HandleFunc("/recruits/recruit/create", controller.CreateCollegeRecruit).Methods("POST")
	// apiRouter.HandleFunc("/recruits/recruit/update/", controller.UpdateCollegeRecruit).Methods("PUT")

	// Requests Controls
	apiRouter.HandleFunc("/admin/requests/fba/", controller.GetFBARequests).Methods("GET")
	apiRouter.HandleFunc("/requests/all/", controller.GetTeamRequests).Methods("GET")
	apiRouter.HandleFunc("/requests/create/", controller.CreateTeamRequest).Methods("POST")
	apiRouter.HandleFunc("/requests/approve/", controller.ApproveTeamRequest).Methods("PUT")
	apiRouter.HandleFunc("/requests/reject/", controller.RejectTeamRequest).Methods("POST")
	apiRouter.HandleFunc("/requests/view/cfb/{teamID}", controller.ViewCFBTeamUponRequest).Methods("GET")
	apiRouter.HandleFunc("/requests/view/nfl/{teamID}", controller.ViewNFLTeamUponRequest).Methods("GET")
	apiRouter.HandleFunc("/requests/remove/{teamID}", controller.RemoveUserFromTeam).Methods("GET")
	apiRouter.HandleFunc("/nfl/requests/all/", controller.GetNFLTeamRequests).Methods("GET")
	apiRouter.HandleFunc("/nfl/requests/create/", controller.CreateNFLTeamRequest).Methods("POST")
	apiRouter.HandleFunc("/nfl/requests/approve/", controller.ApproveNFLTeamRequest).Methods("POST")
	apiRouter.HandleFunc("/nfl/requests/reject/", controller.RejectNFLTeamRequest).Methods("POST")
	apiRouter.HandleFunc("/nfl/requests/remove/{teamID}", controller.RemoveNFLUserFromNFLTeam).Methods("POST")

	// Standings Controls
	cachedApiRouter.HandleFunc("/standings/cfb/season/{seasonID}/", controller.GetAllCollegeStandings).Methods("GET")
	cachedApiRouter.HandleFunc("/standings/cfb/{conferenceID}/{seasonID}/", controller.GetCollegeStandingsByConferenceIDAndSeasonID).Methods("GET")
	cachedApiRouter.HandleFunc("/standings/nfl/season/{seasonID}/", controller.GetAllNFLStandings).Methods("GET")
	cachedApiRouter.HandleFunc("/standings/nfl/{divisionID}/{seasonID}/", controller.GetNFLStandingsByDivisionIDAndSeasonID).Methods("GET")
	cachedApiRouter.HandleFunc("/standings/cfb/history/team/{teamID}/", controller.GetHistoricalRecordsByTeamID).Methods("GET")

	// Stats Controls
	cachedApiRouter.HandleFunc("/stats/cfb/player/{playerID}/season/{seasonID}/", controller.GetCFBSeasonStatsRecord).Methods("GET")
	cachedApiRouter.HandleFunc("/statistics/export/cfb/", controller.ExportCFBStatisticsFromSim).Methods("POST")
	// apiRouter.HandleFunc("/statistics/export/nfl/", controller.ExportNFLStatisticsFromSim).Methods("POST")
	cachedApiRouter.HandleFunc("/statistics/export/players/", controller.ExportPlayerStatsToCSV).Methods("GET")
	cachedApiRouter.HandleFunc("/statistics/export/cfb/{seasonID}/{weekID}/{viewType}/{gameType}", controller.ExportStatsPageContentForSeason).Methods("GET")
	cachedApiRouter.HandleFunc("/statistics/export/nfl/{seasonID}/{weekID}/{viewType}/{gameType}", controller.ExportNFLStatsPageContent).Methods("GET")
	cachedApiRouter.HandleFunc("/statistics/cfb/export/play/by/play/{gameID}", controller.ExportPlayByPlayToCSV).Methods("GET")
	cachedApiRouter.HandleFunc("/statistics/nfl/export/play/by/play/{gameID}", controller.ExportNFLPlayByPlayToCSV).Methods("GET")
	cachedApiRouter.HandleFunc("/statistics/injured/players/", controller.GetInjuryReport).Methods("GET")
	cachedApiRouter.HandleFunc("/statistics/interface/cfb/{seasonID}/{weekID}/{viewType}/{gameType}", controller.GetStatsPageContentForSeason).Methods("GET")
	cachedApiRouter.HandleFunc("/statistics/interface/nfl/{seasonID}/{weekID}/{viewType}/{gameType}", controller.GetNFLStatsPageContent).Methods("GET")
	cachedApiRouter.HandleFunc("/statistics/interface/v2/cfb/{seasonID}/{weekID}/{viewType}/{gameType}", controller.GetCFBStatsPageContent).Methods("GET")
	cachedApiRouter.HandleFunc("/statistics/interface/v2/nfl/{seasonID}/{weekID}/{viewType}/{gameType}", controller.GetProStatsPageContent).Methods("GET")
	// apiRouter.HandleFunc("/statistics/reset/cfb/season/", controller.ResetCFBSeasonalStats).Methods("GET")
	// apiRouter.HandleFunc("/statistics/reset/nfl/season/", controller.ResetNFLSeasonalStats).Methods("GET")

	// Team Controls
	cachedApiRouter.HandleFunc("/teams/college/all/", controller.GetAllCollegeTeams).Methods("GET")
	cachedApiRouter.HandleFunc("/teams/college/data/all/", controller.GetAllCollegeTeamsForRosterPage).Methods("GET")
	cachedApiRouter.HandleFunc("/teams/nfl/all/", controller.GetAllNFLTeams).Methods("GET")
	cachedApiRouter.HandleFunc("/teams/cfb/dashboard/{teamID}/", controller.GetCFBDashboardByTeamID).Methods("GET")
	cachedApiRouter.HandleFunc("/teams/nfl/dashboard/{teamID}/", controller.GetNFLDashboardByTeamID).Methods("GET")
	cachedApiRouter.HandleFunc("/teams/nfl/roster/{teamID}/", controller.GetNFLRecordsForRosterPage).Methods("GET")
	cachedApiRouter.HandleFunc("/teams/college/active/", controller.GetAllActiveCollegeTeams).Methods("GET")
	cachedApiRouter.HandleFunc("/teams/college/available/", controller.GetAllAvailableCollegeTeams).Methods("GET")
	cachedApiRouter.HandleFunc("/teams/college/team/{teamID}/", controller.GetTeamByTeamID).Methods("GET")
	cachedApiRouter.HandleFunc("/teams/college/assign/grades/", controller.AssignCFBTeamGrades).Methods("GET")
	cachedApiRouter.HandleFunc("/teams/nfl/team/{teamID}/", controller.GetNFLTeamByTeamID).Methods("GET")
	cachedApiRouter.HandleFunc("/teams/college/conference/{conferenceID}/", controller.GetTeamsByConferenceID).Methods("GET")
	cachedApiRouter.HandleFunc("/teams/college/division/{divisionID}/", controller.GetTeamsByDivisionID).Methods("GET")
	apiRouter.HandleFunc("/teams/college/update/jersey/", controller.UpdateCFBJersey).Methods("POST")
	apiRouter.HandleFunc("/teams/nfl/update/jersey/", controller.UpdateNFLJersey).Methods("POST")

	// ENGINE CONTROLS
	cachedApiRouter.HandleFunc("/teams/college/sim/{gameID}/", controller.GetHomeAndAwayTeamData).Methods("GET")
	cachedApiRouter.HandleFunc("/teams/nfl/sim/{gameID}/", controller.GetNFLHomeAndAwayTeamData).Methods("GET")

	// TEST Controls
	apiRouter.HandleFunc("/simfba/team/test/{teamID}/{off}/{def}", controller.UpdateIndividualGameplanTEST).Methods("GET")
	apiRouter.HandleFunc("/simfba/mass/test/{off}/{def}", controller.MassUpdateGameplansTEST).Methods("GET")
	apiRouter.HandleFunc("/teams/college/test/sim/{gameID}/", controller.GetHomeAndAwayTeamTestData).Methods("GET")
	// apiRouter.HandleFunc("/simfba/test/cfb/progression/", controller.TestCFBProgressionAlgorithm).Methods("GET")
	// apiRouter.HandleFunc("/simfba/test/nfl/progression/", controller.TestNFLProgressionAlgorithm).Methods("GET")

	// Trade Controls
	apiRouter.HandleFunc("/trades/nfl/all/accepted", controller.GetAllAcceptedTrades).Methods("GET")
	apiRouter.HandleFunc("/trades/nfl/all/rejected", controller.GetAllRejectedTrades).Methods("GET")
	apiRouter.HandleFunc("/trades/nfl/block/{teamID}", controller.GetNFLTradeBlockDataByTeamID).Methods("GET")
	apiRouter.HandleFunc("/trades/nfl/place/block/{playerID}", controller.PlaceNFLPlayerOnTradeBlock).Methods("GET")
	apiRouter.HandleFunc("/trades/nfl/preferences/update", controller.UpdateTradePreferences).Methods("POST")
	apiRouter.HandleFunc("/trades/nfl/create/proposal", controller.CreateNFLTradeProposal).Methods("POST")
	apiRouter.HandleFunc("/trades/nfl/draft/process", controller.SyncTradeFromDraftPage).Methods("POST")
	apiRouter.HandleFunc("/trades/nfl/proposal/accept/{proposalID}", controller.AcceptTradeOffer).Methods("GET")
	apiRouter.HandleFunc("/trades/nfl/proposal/reject/{proposalID}", controller.RejectTradeOffer).Methods("GET")
	apiRouter.HandleFunc("/trades/nfl/proposal/cancel/{proposalID}", controller.CancelTradeOffer).Methods("GET")

	// Training Camp
	apiRouter.HandleFunc("/nfl/training/camp/upload", controller.UploadTrainingCampCSVData).Methods("GET")

	// Transfer Intentions
	apiRouter.HandleFunc("/simfba/sync/transfer/intention", controller.ProcessTransferIntention).Methods("GET")

	// Transfer Intentions
	apiRouter.HandleFunc("/portal/transfer/intention", controller.ProcessTransferIntention).Methods("GET")
	apiRouter.HandleFunc("/portal/transfer/pre/promises", controller.ProcessPrePortalPromises).Methods("GET")
	apiRouter.HandleFunc("/portal/transfer/enter/portal", controller.EnterTheTransferPortal).Methods("GET")
	apiRouter.HandleFunc("/portal/transfer/sync", controller.SyncTransferPortal).Methods("GET")
	apiRouter.HandleFunc("/portal/ai/generate/profiles", controller.FillUpTransferBoardsAI).Methods("GET")
	apiRouter.HandleFunc("/portal/ai/allocate/profiles", controller.AllocateAndPromisePlayersAI).Methods("GET")
	apiRouter.HandleFunc("/portal/page/data/{teamID}", controller.GetTransferPortalPageData).Methods("GET")
	apiRouter.HandleFunc("/portal/profile/create", controller.AddTransferPlayerToBoard).Methods("POST")
	apiRouter.HandleFunc("/portal/profile/remove/{profileID}", controller.RemovePlayerFromTransferPortalBoard).Methods("GET")
	apiRouter.HandleFunc("/portal/saveboard", controller.SaveTransferBoard).Methods("POST")
	apiRouter.HandleFunc("/portal/promise/create", controller.CreatePromise).Methods("POST")
	apiRouter.HandleFunc("/portal/promise/cancel/{promiseID}", controller.CancelPromise).Methods("GET")
	apiRouter.HandleFunc("/portal/promise/player/{playerID}/{teamID}", controller.GetPromiseByPlayerID).Methods("GET")
	apiRouter.HandleFunc("/portal/player/scout/{id}", controller.GetScoutingDataByTransfer).Methods("GET")
	apiRouter.HandleFunc("/portal/export/players/", controller.ExportPortalPlayersToCSV).Methods("GET")

	// Discord Controls
	apiRouter.HandleFunc("/ds/cfb/team/{teamID}/", controller.GetTeamByTeamIDForDiscord).Methods("GET")
	apiRouter.HandleFunc("/ds/college/player/indstats/{id}/{week}/", controller.GetCollegePlayerStatsByNameTeamAndWeek).Methods("GET")
	apiRouter.HandleFunc("/ds/college/player/seasonstats/{id}/", controller.GetCurrentSeasonCollegePlayerStatsByNameTeam).Methods("GET")
	apiRouter.HandleFunc("/ds/college/player/careerstats/{id}/", controller.GetCareerCollegePlayerStatsByID).Methods("GET")
	apiRouter.HandleFunc("/teams/ds/college/week/team/{week}/{team}/", controller.GetWeeklyTeamStatsByTeamAbbrAndWeek).Methods("GET")
	apiRouter.HandleFunc("/teams/ds/college/season/team/{season}/{team}/", controller.GetSeasonTeamStatsByTeamAbbrAndSeason).Methods("GET")
	apiRouter.HandleFunc("/ds/cfb/assign/discord/{teamID}/{discordID}", controller.AssignDiscordIDtoCollegeTeam).Methods("GET")
	apiRouter.HandleFunc("/ds/cfb/player/id/{id}", controller.GetCollegePlayer).Methods("GET")
	apiRouter.HandleFunc("/ds/cfb/player/name/{firstName}/{lastName}/{abbr}", controller.GetCollegePlayerByName).Methods("GET")
	apiRouter.HandleFunc("/ds/nfl/player/id/{id}", controller.GetNFLPlayer).Methods("GET")
	apiRouter.HandleFunc("/ds/nfl/player/name/{firstName}/{lastName}/{abbr}", controller.GetNFLPlayerByName).Methods("GET")
	apiRouter.HandleFunc("/ds/nfl/player/careerstats/{id}", controller.GetNFLPlayerCareer).Methods("GET")
	apiRouter.HandleFunc("/ds/nfl/assign/discord/{teamID}/{discordID}", controller.AssignDiscordIDtoNFLTeam).Methods("GET")
	apiRouter.HandleFunc("/ds/cfb/croots/class/{teamID}/", controller.GetRecruitingClassByTeamID).Methods("GET")
	apiRouter.HandleFunc("/ds/cfb/croot/{id}", controller.GetRecruitViaDiscord).Methods("GET")
	apiRouter.HandleFunc("/schedule/ds/current/week/{league}/", controller.GetCurrentWeekGamesByLeague).Methods("GET")
	apiRouter.HandleFunc("/ds/cfb/flex/{teamOneID}/{teamTwoID}/", controller.CompareCFBTeams).Methods("GET")
	apiRouter.HandleFunc("/ds/nfl/flex/{teamOneID}/{teamTwoID}/", controller.CompareNFLTeams).Methods("GET")
	apiRouter.HandleFunc("/ds/cfb/conference/{conference}/", controller.GetCollegeConferenceStandings).Methods("GET")
	apiRouter.HandleFunc("/ds/cfb/fbs/stream/{timeslot}/{week}/", controller.GetFBSGameStreams).Methods("GET")
	apiRouter.HandleFunc("/ds/cfb/fcs/stream/{timeslot}/{week}/", controller.GetFCSGameStreams).Methods("GET")
	apiRouter.HandleFunc("/ds/nfl/league/stream/{timeslot}/{week}/", controller.GetNFLGameStreams).Methods("GET")

	// Easter Controls
	apiRouter.HandleFunc("/easter/egg/collude/", controller.CollusionButton).Methods("POST")

	// Websocket
	myRouter.HandleFunc("/ws", ws.WebSocketHandler)

	// log.Fatal(http.ListenAndServe(":5001", handler))
	return handlers.CORS(originsOk, headersOk, methodsOk)(myRouter)
}

func loadEnvs() {
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Println("CANNOT LOAD ENV VARIABLES")
	}
}

func handleCron() *cron.Cron {
	c := cron.New()
	runJobs := os.Getenv("RUN_JOBS")
	if runJobs != "false" {
		// Fill AI Recruiting Boards
		c.AddFunc("0 5 * * 4", controller.FillAIBoardsViaCron)
		// Update AI Gameplans and DCs
		c.AddFunc("0 1 * * 3", controller.RunAISchemeAndDCViaCron)
		c.AddFunc("0 4 * * 3", controller.RunAIGameplanViaCron)
		// Allocate AI Boards
		c.AddFunc("0 3 * * 4,6", controller.SyncAIBoardsViaCron)
		// Run RES
		c.AddFunc("0 7 * * 4", controller.RunRESViaCron)
		// Sync Recruiting
		c.AddFunc("0 7 * * 3", controller.SyncRecruitingViaCron)
		// Sync Free Agency
		c.AddFunc("0 16 * * *", controller.SyncFreeAgencyViaCron)
		// Sync Extension Offers
		// Run the Games
		c.AddFunc("0 4 * * 4", controller.RunTheGamesViaCron)
		// Reveal Timeslot Results
		c.AddFunc("0 21 * * 4", controller.ShowCFBThursdayViaCron) // Thurs Night
		c.AddFunc("0 20 * * 4", controller.ShowNFLThursdayViaCron) // Thurs NFL
		c.AddFunc("0 21 * * 5", controller.ShowCFBFridayViaCron)   // Fri Night
		c.AddFunc("0 15 * * 6", controller.ShowCFBSatMornViaCron)  // Sat. Morning
		c.AddFunc("0 17 * * 6", controller.ShowCFBSatAftViaCron)   // Sat. Afternoon
		c.AddFunc("0 19 * * 6", controller.ShowCFBSatEveViaCron)   // Sat. Evening
		c.AddFunc("0 21 * * 6", controller.ShowCFBSatNitViaCron)   // Sat. Night
		c.AddFunc("0 15 * * 0", controller.ShowNFLSunNoonViaCron)  // Sun Noon
		c.AddFunc("0 17 * * 0", controller.ShowNFLSunAftViaCron)   // Sun Aft
		c.AddFunc("0 19 * * 0", controller.ShowNFLSunNitViaCron)   // Sun Nit
		c.AddFunc("0 17 * * 1", controller.ShowNFLMonNitViaCron)   // Mon Nit
		// Sync Week
		c.AddFunc("0 18 * * 1", controller.SyncToNextWeekViaCron)
	}

	c.Start()

	return c
}

func main() {
	loadEnvs()
	InitialMigration()

	fmt.Println("Setting up polling...")
	go monitorDBForUpdates()

	fmt.Println("Loading cron...")
	cronJobs := handleCron()
	fmt.Println("Loading Handler Requests.")
	fmt.Println("Football Server Initialized.")
	srv := &http.Server{
		Addr:    ":5001",
		Handler: handleRequests(),
	}

	go func() {
		fmt.Println("Server starting on port 5001")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not listen on %s: %v\n", srv.Addr, err)
		}
	}()

	// Create a channel to listen for system interrupts (Ctrl+C, etc.)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// Block until a signal is received
	<-quit
	fmt.Println("Shutting down server...")

	// Gracefully shutdown the server with a timeout of 5 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Stop cron jobs
	cronJobs.Stop()

	// Shutdown the server
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	fmt.Println("Server exiting")
}
