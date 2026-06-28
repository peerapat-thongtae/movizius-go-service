package tvmaze

import "time"

// ScheduleEntry is a single episode from the TVMaze /schedule/full response.
type ScheduleEntry struct {
	Season   int       `json:"season"`
	Number   int       `json:"number"`
	Airstamp time.Time `json:"airstamp"`
	Embedded struct {
		Show struct {
			Externals struct {
				Imdb *string `json:"imdb"`
			} `json:"externals"`
		} `json:"show"`
	} `json:"_embedded"`
}
