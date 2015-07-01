package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type weatherProvider interface {
	temperature(city string) (float64, error) // in Kelvin
}

type Coord struct {
	Lon float64
	Lat float64
}
type multiWeatherProvider []weatherProvider
type openWeatherMap struct{}
type weatherUnderground struct {
	apiKey string
}
type forecastIo struct {
	apiKey string
}

func FloatToString(input_num float64) string {
	return strconv.FormatFloat(input_num, 'f', 2, 64)
}

func FahrenheitToCelsius(input_num float64) float64 {
	result := (input_num - 32) * 5 / 9
	return result
}

func (w forecastIo) temperature(city string) (float64, error) {
	coord, err := openWeatherMap{}.coordinates(city)
	if err != nil {
		return 0, err
	}

	url := "https://api.forecast.io/forecast/" + w.apiKey + "/" + FloatToString(coord.Lat) + "," + FloatToString(coord.Lon)

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	var d struct {
		Currently struct {
			Fahrenheit float64 `json:"temperature"`
		} `json:"currently"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	log.Printf("forecastIo: %s: %.2f", city, FahrenheitToCelsius(d.Currently.Fahrenheit))
	return FahrenheitToCelsius(d.Currently.Fahrenheit), nil
}

func (w openWeatherMap) coordinates(city string) (Coord, error) {
	resp, err := http.Get("http://api.openweathermap.org/data/2.5/weather?q=" + city)
	if err != nil {
		return Coord{}, nil
	}

	defer resp.Body.Close()

	var d struct {
		Coord struct {
			Lon float64 `json:"lon"`
			Lat float64 `json:"lat"`
		} `json:"coord"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return Coord{}, nil
	}

	return Coord{d.Coord.Lon, d.Coord.Lat}, nil
}

func (w openWeatherMap) temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.openweathermap.org/data/2.5/weather?q=" + city)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	var d struct {
		Main struct {
			Kelvin float64 `json:"temp"`
		} `json:"main"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	celsius := d.Main.Kelvin - 273.15
	log.Printf("openWeatherMap: %s: %.2f", city, celsius)
	return celsius, nil
}

func (w weatherUnderground) temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.wunderground.com/api/" + w.apiKey + "/conditions/q/" + city + ".json")
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	var d struct {
		Observation struct {
			Celsius float64 `json:"temp_c"`
		} `json:"current_observation"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	log.Printf("weatherUnderground: %s, %.2f", city, d.Observation.Celsius)
	return d.Observation.Celsius, err
}

func (w multiWeatherProvider) temperature(city string) (float64, error) {
	temps := make(chan float64, len(w))
	errs := make(chan error, len(w))

	for _, provider := range w {
		go func(p weatherProvider) {
			k, err := p.temperature(city)
			if err != nil {
				errs <- err
				return
			}
			temps <- k
		}(provider)
	}

	sum := 0.0

	for i := 0; i < len(w); i++ {
		select {
		case temp := <-temps:
			sum += temp

		case err := <-errs:
			return 0, err
		}
	}

	return sum / float64(len(w)), nil
}

func main() {

	http.HandleFunc("/", hello)
	http.HandleFunc("/coordinates/", coordinates)
	http.HandleFunc("/weather/", weather)

	http.ListenAndServe(":8080", nil)
}

func hello(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello!"))
}

func coordinates(w http.ResponseWriter, r *http.Request) {
	city := strings.SplitN(r.URL.Path, "/", 3)[2]

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	lat, err := openWeatherMap{}.coordinates(city)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"city": city,
		"temp": lat,
	})
}

func weather(w http.ResponseWriter, r *http.Request) {
	mw := multiWeatherProvider{
		openWeatherMap{},
		weatherUnderground{apiKey: "1df429f462bc7ee1"},
		forecastIo{apiKey: "12e03ff21975540f37c2b8cc79e3093b"},
	}

	begin := time.Now()
	city := strings.SplitN(r.URL.Path, "/", 3)[2]

	temp, err := mw.temperature(city)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"city": city,
		"temp": temp,
		"took": time.Since(begin).String(),
	})
}
