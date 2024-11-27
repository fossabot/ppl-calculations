package ports

import (
	"context"
	"errors"
	"github.com/sirupsen/logrus"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"ppl-calculations/adapters"
	"ppl-calculations/adapters/templator/models"
	"ppl-calculations/adapters/templator/parsing"
	"ppl-calculations/app"
	"ppl-calculations/app/queries"
	"ppl-calculations/domain/callsign"
	"ppl-calculations/domain/weight_balance"
	"strconv"
	"sync"
	"time"
)

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func NewHTTPListener(ctx context.Context, wg *sync.WaitGroup, app app.Application, assets fs.FS) {
	mux := http.NewServeMux()

	templatesFS, err := fs.Sub(assets, "assets/templates")
	if err != nil {
		logrus.WithError(err).Fatal("cannot open asset templates")
	}

	tmpl, err := template.New("base").Funcs(template.FuncMap{
		"derefString": derefString,
		"mod": func(i, j int) bool {
			return i%j != 0
		},
	}).ParseFS(templatesFS, "*.html")
	if err != nil {
		logrus.WithError(err).Fatal("cannot parse asset templates")
	}

	cssFs, err := fs.Sub(assets, "assets/css")
	if err != nil {
		log.Fatalf("Fout bij het parsen van css: %v", err)
	}

	mux.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.FS(cssFs))))

	mux.HandleFunc("/aquila-wb", func(w http.ResponseWriter, r *http.Request) {
		urlTakeOffMass := r.URL.Query().Get("takeoff-mass")
		urlTakeOffMassMoment := r.URL.Query().Get("takeoff-mass-moment")
		urlLandingMass := r.URL.Query().Get("landing-mass")
		urlLandingMassMoment := r.URL.Query().Get("landing-mass-moment")
		urlLimits := r.URL.Query().Get("limits")
		urlCallSign := r.URL.Query().Get("callsign")

		if urlTakeOffMass == "" || urlTakeOffMassMoment == "" || urlLandingMass == "" || urlLandingMassMoment == "" || urlLimits == "" || urlCallSign == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		takeOffMass, err := strconv.ParseFloat(urlTakeOffMass, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		takeOffMassMoment, err := strconv.ParseFloat(urlTakeOffMassMoment, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		landingMass, err := strconv.ParseFloat(urlLandingMass, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		landingMassMoment, err := strconv.ParseFloat(urlLandingMassMoment, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		limits := false
		if urlLimits == "true" {
			limits = true
		}

		cs, err := callsign.New(urlCallSign)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		chart, err := app.Queries.WBChart.Handle(r.Context(), queries.WBChartRequest{
			CallSign:          cs,
			TakeOffMassMoment: *weight_balance.NewMassMoment("Take-off", takeOffMassMoment/takeOffMass, weight_balance.NewMass(takeOffMass)),
			LandingMassMoment: *weight_balance.NewMassMoment("Landing", landingMassMoment/landingMass, weight_balance.NewMass(landingMass)),
			WithinLimits:      limits,
		})

		w.Header().Set("Content-Type", "image/svg+xml")

		_, err = io.Copy(w, chart)
		if err != nil {
			logrus.WithError(err).Error("writing chart")
		}
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")

		stateService, err := adapters.NewCookieStateService(w, r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		switch r.Method {
		case http.MethodGet:
			if r.Header.Get("HX-Request") == "true" && r.URL.Query().Get("submit") == "weight" {
				loadSheet, err := app.Queries.LoadSheet.Handle(r.Context(), stateService)
				if err != nil {
					logrus.WithError(err).Error("reading loadsheet")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				w.Header().Set("HX-Push-Url", "/")
				if err = tmpl.ExecuteTemplate(w, "wb_form", models.WeightFromLoadSheet(loadSheet)); err != nil {
					logrus.WithError(err).Error("executing template")
				}
			} else if r.Header.Get("HX-Request") == "true" && r.URL.Query().Get("submit") == "Volgende" {
				updateLoadSheetRequest, err := parsing.UpdateLoadSheetRequest(r)
				if err != nil {
					logrus.WithError(err).Error("creating update loadsheet request")
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				if err = app.Commands.UpdateLoadSheet.Handle(r.Context(), stateService, updateLoadSheetRequest); err != nil {
					logrus.WithError(err).Error("update loadsheet")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				loadSheet, err := app.Queries.LoadSheet.Handle(r.Context(), stateService)
				if err != nil {
					logrus.WithError(err).Error("reading loadsheet")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				w.Header().Set("HX-Push-Url", "/fuel")
				if err = tmpl.ExecuteTemplate(w, "fuel_form", models.WeightFromLoadSheet(loadSheet)); err != nil {
					logrus.WithError(err).Error("executing template")
				}
			} else {
				loadSheet, err := app.Queries.LoadSheet.Handle(r.Context(), stateService)
				if err != nil {
					logrus.WithError(err).Error("reading loadsheet")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				if err = tmpl.ExecuteTemplate(w, "index.html", models.WeightFromLoadSheet(loadSheet)); err != nil {
					logrus.WithError(err).Error("executing template")
				}
			}
		default:
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/fuel", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")

		stateService, err := adapters.NewCookieStateService(w, r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		switch r.Method {
		case http.MethodGet:

			if r.Header.Get("HX-Request") == "true" && r.URL.Query().Get("submit") == "Vorige" {
				loadSheet, err := app.Queries.LoadSheet.Handle(r.Context(), stateService)
				if err != nil {
					logrus.WithError(err).Error("reading loadsheet")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				w.Header().Set("HX-Push-Url", "/")
				if err = tmpl.ExecuteTemplate(w, "wb_form", models.WeightFromLoadSheet(loadSheet)); err != nil {
					logrus.WithError(err).Error("executing template")
				}
			} else if r.Header.Get("HX-Request") == "true" && r.URL.Query().Get("submit") == "Volgende" {
				updateFuelSheetRequest, err := parsing.UpdateFuelSheetRequest(r)
				if err != nil {
					logrus.WithError(err).Error("creating update fuelsheet request")
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				if err = app.Commands.UpdateFuelSheet.Handle(r.Context(), stateService, updateFuelSheetRequest); err != nil {
					logrus.WithError(err).Error("update loadsheet")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				w.Header().Set("HX-Push-Url", "/stats")
				_ = tmpl.ExecuteTemplate(w, "calculations_form", models.StatsFromState(*s))
			} else {
				fuelSheet, err := app.Queries.FuelSheet.Handle(r.Context(), stateService)
				if err != nil {
					http.Redirect(w, r, "/", http.StatusSeeOther)
					return
				}

				if err = tmpl.ExecuteTemplate(w, "index.html", models.FuelFromFuelSheet(fuelSheet)); err != nil {
					logrus.WithError(err).Error("parsing template")
				}
			}
		default:
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}

		//s, err := parsing.NewFromFuelRequest(r)
		//if err != nil {
		//	http.Redirect(w, r, "/", http.StatusSeeOther)
		//	return
		//}
		//
		////_ = parsing.WriteState(s, w)
		//
		//if r.Header.Get("HX-Request") == "true" && r.URL.Query().Get("submit") == "Vorige" {
		//	w.Header().Set("HX-Push-Url", "/")
		//	_ = tmpl.ExecuteTemplate(w, "wb_form", models.WeightFromState(*s))
		//} else if r.Header.Get("HX-Request") == "true" && r.URL.Query().Get("submit") == "Volgende" {
		//	w.Header().Set("HX-Push-Url", "/stats")
		//	_ = tmpl.ExecuteTemplate(w, "calculations_form", models.StatsFromState(*s))
		//} else {
		//	_ = tmpl.ExecuteTemplate(w, "index.html", models.FuelFromState(*s))
		//}
	})

	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")

		s, err := parsing.NewFromStatsRequest(r)
		if err != nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		//_ = parsing.WriteState(s, w)

		if r.Header.Get("HX-Request") == "true" && r.URL.Query().Get("submit") == "Vorige" {
			w.Header().Set("HX-Push-Url", "/fuel")
			_ = tmpl.ExecuteTemplate(w, "fuel_form", models.FuelFromState(*s))
		} else if r.Header.Get("HX-Request") == "true" && r.URL.Query().Get("submit") == "Volgende" {
			w.Header().Set("HX-Push-Url", "/export")
			_ = tmpl.ExecuteTemplate(w, "calculations_form", models.StatsFromState(*s))
		} else {
			_ = tmpl.ExecuteTemplate(w, "index.html", models.StatsFromState(*s))
		}
	})

	mux.HandleFunc("/wind-option", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.Method {
		case http.MethodGet:
			stateService, err := adapters.NewCookieStateService(w, r)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			s, err := stateService.State()
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			if err = parsing.ParseWeightRequest(r, s); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			if err = tmpl.ExecuteTemplate(w, "wb_form_wind_option", models.WeightFromState(*s)); err != nil {
				logrus.WithError(err).Error("executing template")
			}
		default:
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/fuel-option", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")

		s, err := parsing.NewFromFuelRequest(r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		err = tmpl.ExecuteTemplate(w, "fuel_form_max_fuel_option", models.FuelFromState(*s))
	})

	server := &http.Server{
		Addr:    ":80",
		Handler: mux,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		logrus.WithField("port", 80).Info("starting HTTP server")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logrus.WithError(err).Error("error starting HTTP server")
		}
		logrus.Info("HTTP server closed")
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logrus.WithError(err).Error("error shutting down HTTP server")
		}
	}
}
