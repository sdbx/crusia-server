package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/cors"
	"github.com/go-chi/chi/middleware"
	"github.com/sdbx/crusia-server/store"
	"github.com/sdbx/crusia-server/utils"
)

type ApiInterface interface {
	DecryptSaveData(version int, payload string) ([]byte, error)
	CreateToken(id int) (string, error)
	GetToken(tok string) (int, error)
	GetStore() store.Store
	GetVersion() int
}

type Api struct {
	in ApiInterface
}

func New(in ApiInterface) *Api {
	return &Api{in: in}
}

func (a *Api) Http() http.Handler {
	r := chi.NewRouter()
	cors := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Accept", "X-Authorization", "Content-Type", "X-Save-Version"},
		AllowCredentials: true,
		MaxAge:           300,
	})
	r.Use(cors.Handler)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Get("/crossdomain.xml", getCrossDomain)
	r.Get("/version", a.GetVersion)
	r.Post("/login", a.Login)
	r.Post("/register", a.Register)
	r.Route("/save", func(s chi.Router) {
		s.Use(a.UserMiddleWare)
		s.Post("/get", a.GetSaveData)
		s.Post("/set", a.PostSaveData)
	})

	return r
}

func getCrossDomain(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, `
<?xml version="1.0" ?>
<cross-domain-policy>
  <site-control permitted-cross-domain-policies="master-only"/>
  <allow-access-from domain="*"/>
  <allow-http-request-headers-from domain="*" headers="*"/>
</cross-domain-policy>
`)
}

func (a *Api) GetVersion(w http.ResponseWriter, r *http.Request) {
	v := a.in.GetVersion()
	utils.HttpJson(w, v)
}

func (a *Api) Login(w http.ResponseWriter, r *http.Request) {
	req := struct {
		Username string `json:"username"`
		Passhash string `json:"passhash"`
	}{}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		utils.HttpError(w, err, 400)
		return
	}

	u, err := a.in.GetStore().GetUserByUsername(req.Username)
	if err != nil {
		utils.HttpError(w, err, 404)
		return
	}

	if u.Passhash != req.Passhash {
		utils.HttpError(w, fmt.Errorf(""), 403)
		return
	}

	tok, err := a.in.CreateToken(u.ID)
	if err != nil {
		utils.HttpError(w, err, 500)
		return
	}

	utils.HttpJson(w, tok)
}

func (a *Api) Register(w http.ResponseWriter, r *http.Request) {
	req := struct {
		Username string `json:"username"`
		Passhash string `json:"passhash"`
	}{}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		utils.HttpError(w, err, 400)
		return
	}

	_, err = a.in.GetStore().GetUserByUsername(req.Username)
	if err == nil {
		utils.HttpError(w, fmt.Errorf(""), 409)
		return
	}

	u := &store.User{
		Username: req.Username,
		Passhash: req.Passhash,
	}
	err = a.in.GetStore().CreateUser(u)
	if err != nil {
		utils.HttpError(w, err, 409)
		return
	}

	err = a.in.GetStore().CreateSaveData(&store.SaveData{
		UserID:  u.ID,
		Edited:  time.Now(),
		Payload: "{}",
	})
	if err != nil {
		utils.HttpError(w, err, 500)
		return
	}

	utils.HttpOk(w)
}

func (a *Api) GetSaveData(w http.ResponseWriter, r *http.Request) {
	u := getUser(r)
	data, err := a.in.GetStore().GetSaveData(u.ID)
	if err != nil {
		utils.HttpError(w, err, 500)
		return
	}
	utils.HttpJson(w, data.Payload)
}

func (a *Api) PostSaveData(w http.ResponseWriter, r *http.Request) {
	ver_ := r.Header.Get("X-Save-Version")
	ver, err := strconv.Atoi(ver_)
	if err != nil {
		utils.HttpError(w, err, 400)
		return
	}

	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		utils.HttpError(w, err, 400)
		return
	}

	buf, err := a.in.DecryptSaveData(ver, string(payload))
	if err != nil {
		utils.HttpError(w, err, 400)
		return
	}

	u := getUser(r)
	data := &store.SaveData{
		UserID:  u.ID,
		Edited:  time.Now(),
		Payload: string(buf),
	}

	err = a.in.GetStore().UpdateSaveData(data)
	if err != nil {
		utils.HttpError(w, err, 500)
		return
	}
	utils.HttpOk(w)
}
