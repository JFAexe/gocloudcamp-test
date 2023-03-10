package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"gocloudcamp_test/internal/database"
	"gocloudcamp_test/internal/service"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
)

var (
	ErrParseId         = errors.New("can't parse id")
	ErrRequestBody     = errors.New("there is an error in the request body")
	ErrNoSongsProvided = errors.New("no songs provided")
)

func New(ctx context.Context, s *service.Service) http.Handler {
	router := chi.NewRouter()

	router.Use(requestLogger())
	router.Use(middleware.StripSlashes)
	router.Use(render.SetContentType(render.ContentTypeJSON))

	router.NotFound(notFound)
	router.MethodNotAllowed(notAllowed)

	router.Get("/ping", ping)

	router.Route("/v1", func(v1 chi.Router) {
		v1.Route("/playlist", func(pl chi.Router) {
			pl.Get("/", getAll(s))
			pl.Get("/{id}", getPlaylist(s))

			pl.Post("/", newPlaylist(s))
			pl.Patch("/{id}/name", namePlaylist(s))
			pl.Patch("/{id}/time", timePlaylist(s))
			pl.Delete("/{id}", deletePlaylist(s))

			pl.Post("/{id}/launch", launchPlaylist(ctx, s))
			pl.Post("/{id}/stop", stopPlaylist(s))

			pl.Post("/{id}/play", playPlaylist(s))
			pl.Post("/{id}/pause", pausePlaylist(s))
			pl.Post("/{id}/next", nextPlaylist(s))
			pl.Post("/{id}/prev", prevPlaylist(s))

			pl.Post("/{id}/song", addSong(s))
			pl.Patch("/{id}/song/{sid}", editSong(s))
			pl.Delete("/{id}/song/{sid}", removeSong(s))
		})
	})

	return router
}

func notFound(w http.ResponseWriter, r *http.Request) {
	render.Render(w, r, responseNotFound)
}

func notAllowed(w http.ResponseWriter, r *http.Request) {
	render.Render(w, r, responseNotAllowed)
}

func ping(w http.ResponseWriter, r *http.Request) {
	render.Render(w, r, &messageResponse{
		HTTPStatusCode: http.StatusOK,
		MessageText:    "pong",
	})
}

func parseId(r *http.Request, s string) (uint, error) {
	id, err := strconv.ParseUint(chi.URLParam(r, s), 10, 32)
	if err != nil {
		return 0, ErrParseId
	}

	return uint(id), nil
}

func getAll(s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var pls []playlistData

		for _, pl := range s.GetPlaylists() {
			pls = append(pls, playlistData{
				Status: pl.Status(),
				Songs:  pl.GetSongsList(),
			})
		}

		render.Render(w, r, &allResponse{
			HTTPStatusCode: http.StatusOK,
			Playlists:      pls,
		})
	}
}

func getPlaylist(s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseId(r, "id")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		pl, err := s.GetPlaylist(id)
		if err != nil {
			render.Render(w, r, responseInternalError(err))

			return
		}

		render.Render(w, r, &playlistResponse{
			HTTPStatusCode: http.StatusOK,
			Playlist: playlistData{
				Status: pl.Status(),
				Songs:  pl.GetSongsList(),
			},
		})
	}
}

func newPlaylist(s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var data struct {
			Name  string
			Songs []database.Song
		}

		err := dec.Decode(&data)
		if err != nil {
			render.Render(w, r, responseInvalidRequest(ErrRequestBody))

			return
		}

		var pl database.Playlist
		pl.Name = data.Name

		if err := s.CreatePlaylist(&pl); err != nil {
			render.Render(w, r, responseInternalError(err))

			s.ChanErrorLog <- err

			return
		}

		id := pl.Id

		for _, sn := range data.Songs {
			sn.PlaylistId = id

			if err := s.CreateSong(&sn); err != nil {
				render.Render(w, r, responseInternalError(err))

				s.ChanErrorLog <- err

				return
			}
		}

		render.Render(w, r, &messageResponse{
			HTTPStatusCode: http.StatusCreated,
			MessageText:    "playlist created",
			PlaylistId:     id,
		})
	}
}

func deletePlaylist(s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseId(r, "id")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		_, err = s.GetPlaylist(id)
		if err != nil {
			render.Render(w, r, responseInternalError(err))

			return
		}

		if err := s.DeletePlaylist(id); err != nil {
			render.Render(w, r, responseInternalError(err))

			s.ChanErrorLog <- err

			return
		}

		render.Render(w, r, &messageResponse{
			HTTPStatusCode: http.StatusOK,
			MessageText:    "playlist deleted",
			PlaylistId:     id,
		})
	}
}

func addSong(s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var data []database.Song

		err := dec.Decode(&data)
		if err != nil {
			render.Render(w, r, responseInvalidRequest(ErrRequestBody))

			return
		}

		if len(data) < 1 {
			render.Render(w, r, responseInvalidRequest(ErrNoSongsProvided))

			return
		}

		id, err := parseId(r, "id")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		_, err = s.GetPlaylist(id)
		if err != nil {
			render.Render(w, r, responseInternalError(err))

			return
		}

		for _, sn := range data {
			sn.PlaylistId = id

			if err := s.CreateSong(&sn); err != nil {
				render.Render(w, r, responseInternalError(err))

				s.ChanErrorLog <- err

				return
			}
		}

		render.Render(w, r, &messageResponse{
			HTTPStatusCode: http.StatusCreated,
			MessageText:    "songs added",
			PlaylistId:     id,
		})
	}
}

func editSong(s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var data database.Song

		err := dec.Decode(&data)
		if err != nil {
			render.Render(w, r, responseInvalidRequest(ErrRequestBody))

			return
		}

		id, err := parseId(r, "id")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		sid, err := parseId(r, "sid")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		if err := s.EditSong(id, sid, data.Name, data.Duration); err != nil {
			render.Render(w, r, responseInternalError(err))

			s.ChanErrorLog <- err

			return
		}

		render.Render(w, r, &messageResponse{
			HTTPStatusCode: http.StatusOK,
			MessageText:    "song updated",
			PlaylistId:     id,
		})
	}
}

func removeSong(s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseId(r, "id")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		sid, err := parseId(r, "sid")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		if err := s.DeleteSong(id, sid); err != nil {
			render.Render(w, r, responseInternalError(err))

			s.ChanErrorLog <- err

			return
		}

		render.Render(w, r, &messageResponse{
			HTTPStatusCode: http.StatusOK,
			MessageText:    "song removed",
			PlaylistId:     id,
		})
	}
}

func playPlaylist(s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseId(r, "id")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		pl, err := s.GetPlaylist(id)
		if err != nil {
			render.Render(w, r, responseInternalError(err))

			return
		}

		if err = pl.Play(); err != nil {
			render.Render(w, r, responseInternalError(err))

			s.ChanErrorLog <- err

			return
		}

		render.Render(w, r, &messageResponse{
			HTTPStatusCode: http.StatusOK,
			MessageText:    "playlist is playing",
			PlaylistId:     id,
		})
	}
}

func pausePlaylist(s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseId(r, "id")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		pl, err := s.GetPlaylist(id)
		if err != nil {
			render.Render(w, r, responseInternalError(err))

			return
		}

		if err = pl.Pause(); err != nil {
			render.Render(w, r, responseInternalError(err))

			s.ChanErrorLog <- err

			return
		}

		render.Render(w, r, &messageResponse{
			HTTPStatusCode: http.StatusOK,
			MessageText:    "playlist paused",
			PlaylistId:     id,
		})
	}
}

func nextPlaylist(s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseId(r, "id")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		pl, err := s.GetPlaylist(id)
		if err != nil {
			render.Render(w, r, responseInternalError(err))

			return
		}

		if err = pl.Next(); err != nil {
			render.Render(w, r, responseInternalError(err))

			s.ChanErrorLog <- err

			return
		}

		render.Render(w, r, &messageResponse{
			HTTPStatusCode: http.StatusOK,
			MessageText:    "playlist switched to next song",
			PlaylistId:     id,
		})
	}
}

func prevPlaylist(s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseId(r, "id")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		pl, err := s.GetPlaylist(id)
		if err != nil {
			render.Render(w, r, responseInternalError(err))

			return
		}

		if err = pl.Prev(); err != nil {
			render.Render(w, r, responseInternalError(err))

			s.ChanErrorLog <- err

			return
		}

		render.Render(w, r, &messageResponse{
			HTTPStatusCode: http.StatusOK,
			MessageText:    "playlist switched to prev song",
			PlaylistId:     id,
		})
	}
}

func launchPlaylist(ctx context.Context, s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseId(r, "id")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		_, err = s.GetPlaylist(id)
		if err != nil {
			render.Render(w, r, responseInternalError(err))

			return
		}

		if err = s.LaunchPlaylist(ctx, id); err != nil {
			render.Render(w, r, responseInternalError(err))

			s.ChanErrorLog <- err

			return
		}

		render.Render(w, r, &messageResponse{
			HTTPStatusCode: http.StatusOK,
			MessageText:    "playlist is processing",
			PlaylistId:     id,
		})
	}
}

func stopPlaylist(s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseId(r, "id")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		pl, err := s.GetPlaylist(id)
		if err != nil {
			render.Render(w, r, responseInternalError(err))

			return
		}

		if err = pl.Stop(); err != nil {
			render.Render(w, r, responseInternalError(err))

			s.ChanErrorLog <- err

			return
		}

		render.Render(w, r, &messageResponse{
			HTTPStatusCode: http.StatusOK,
			MessageText:    "playlist stopped",
			PlaylistId:     id,
		})
	}
}

func namePlaylist(s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var data struct{ Name string }

		err := dec.Decode(&data)
		if err != nil {
			render.Render(w, r, responseInvalidRequest(ErrRequestBody))

			return
		}

		id, err := parseId(r, "id")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		_, err = s.GetPlaylist(id)
		if err != nil {
			render.Render(w, r, responseInternalError(err))

			return
		}

		if err = s.EditPlaylist(id, data.Name); err != nil {
			render.Render(w, r, responseInternalError(err))

			s.ChanErrorLog <- err

			return
		}

		render.Render(w, r, &messageResponse{
			HTTPStatusCode: http.StatusOK,
			MessageText:    "playlist renamed",
			PlaylistId:     id,
		})
	}
}

func timePlaylist(s *service.Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var data struct{ Time uint }

		err := dec.Decode(&data)
		if err != nil {
			render.Render(w, r, responseInvalidRequest(ErrRequestBody))

			return
		}

		id, err := parseId(r, "id")
		if err != nil {
			render.Render(w, r, responseInvalidRequest(err))

			return
		}

		pl, err := s.GetPlaylist(id)
		if err != nil {
			render.Render(w, r, responseInternalError(err))

			return
		}

		if err = pl.SetTime(data.Time); err != nil {
			render.Render(w, r, responseInternalError(err))

			s.ChanErrorLog <- err

			return
		}

		render.Render(w, r, &messageResponse{
			HTTPStatusCode: http.StatusOK,
			MessageText:    "playlist time set",
			PlaylistId:     id,
		})
	}
}
