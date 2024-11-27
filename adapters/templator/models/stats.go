package models

import (
	"fmt"
	"ppl-calculations/domain/calculations"
	"ppl-calculations/domain/fuel"
	"ppl-calculations/domain/state"
	"strings"
)

type WeightAndBalanceItem struct {
	Name       string
	LeverArm   string
	Mass       string
	MassMoment string
}

type WeightAndBalanceState struct {
	Items        []WeightAndBalanceItem
	Total        WeightAndBalanceItem
	WithinLimits bool
}

type Stats struct {
	Base

	FuelTaxi        string
	FuelTrip        string
	FuelAlternate   string
	FuelContingency string
	FuelReserve     string
	FuelTotal       string
	FuelExtra       string
	FuelExtraAbs    string
	FuelSufficient  bool

	ChartUrl string

	WeightAndBalanceTakeOff WeightAndBalanceState
	WeightAndBalanceLanding WeightAndBalanceState
}

func parseNumber(number string) string {
	return strings.ReplaceAll(number, ".", ",")
}

func StatsFromState(s state.State) interface{} {
	template := Stats{
		Base: Base{
			Step: string(StepStats),
		},
	}

	var f fuel.Fuel
	var err error
	var wb *calculations.WeightBalance
	if s.MaxFuel != nil && *s.MaxFuel {
		wb, f, err = calculations.NewWeightAndBalanceMaxFuel(*s.CallSign, *s.Pilot, *s.PilotSeat, s.Passenger, s.PassengerSeat, *s.Baggage, *s.FuelType)
		if err != nil {
			panic(err)
		}
	} else {
		f = *s.Fuel
		wb, err = calculations.NewWeightAndBalance(*s.CallSign, *s.Pilot, *s.PilotSeat, s.Passenger, s.PassengerSeat, *s.Baggage, f)
		if err != nil {
			panic(err)
		}
	}

	planning, err := calculations.NewFuelPlanning(*s.TripDuration, *s.AlternateDuration, f)
	if err != nil {
		panic(err)
	}

	landingWb, err := calculations.NewWeightAndBalance(*s.CallSign, *s.Pilot, *s.PilotSeat, s.Passenger, s.PassengerSeat, *s.Baggage, fuel.Subtract(f, planning.Trip, planning.Taxi))
	if err != nil {
		panic(err)
	}

	template.FuelSufficient = planning.Sufficient
	template.FuelTaxi = parseNumber(planning.Taxi.Volume.String(*s.FuelVolumeType))
	template.FuelTrip = parseNumber(planning.Trip.Volume.String(*s.FuelVolumeType))
	template.FuelAlternate = parseNumber(planning.Alternate.Volume.String(*s.FuelVolumeType))
	template.FuelContingency = parseNumber(planning.Contingency.Volume.String(*s.FuelVolumeType))
	template.FuelReserve = parseNumber(planning.Reserve.Volume.String(*s.FuelVolumeType))
	template.FuelTotal = parseNumber(planning.Total.Volume.String(*s.FuelVolumeType))
	template.FuelExtra = parseNumber(planning.Extra.Volume.String(*s.FuelVolumeType))
	template.FuelExtraAbs = parseNumber(planning.Extra.Volume.String(*s.FuelVolumeType))

	wbState := WeightAndBalanceState{}
	for _, i := range wb.Moments {
		m := parseNumber(fmt.Sprintf("%.2f", i.Mass.Kilo()))
		if strings.HasPrefix(i.Name, "Fuel") {
			m = fmt.Sprintf("(%s) %s", template.FuelTotal, m)
		}

		wbState.Items = append(wbState.Items, WeightAndBalanceItem{
			Name:       parseNumber(i.Name),
			LeverArm:   parseNumber(fmt.Sprintf("%.4f", i.Arm)),
			Mass:       m,
			MassMoment: parseNumber(fmt.Sprintf("%.2f", i.KGM())),
		})
	}

	wbState.Total = WeightAndBalanceItem{
		Name:       parseNumber(wb.Total.Name),
		LeverArm:   parseNumber(fmt.Sprintf("%.4f", wb.Total.Arm)),
		Mass:       parseNumber(fmt.Sprintf("%.2f", wb.Total.Mass.Kilo())),
		MassMoment: parseNumber(fmt.Sprintf("%.2f", wb.Total.KGM())),
	}

	wbState.WithinLimits = wb.WithinLimits

	wbLandingState := WeightAndBalanceState{}

	for _, i := range landingWb.Moments {
		m := parseNumber(fmt.Sprintf("%.2f", i.Mass.Kilo()))
		if strings.HasPrefix(i.Name, "Fuel") {
			m = fmt.Sprintf("(%s) %s", parseNumber(planning.Total.Volume.Subtract(planning.Trip.Volume).String(*s.FuelVolumeType)), m)
		}

		wbLandingState.Items = append(wbLandingState.Items, WeightAndBalanceItem{
			Name:       parseNumber(i.Name),
			LeverArm:   parseNumber(fmt.Sprintf("%.4f", i.Arm)),
			Mass:       m,
			MassMoment: parseNumber(fmt.Sprintf("%.2f", i.KGM())),
		})
	}

	wbLandingState.Total = WeightAndBalanceItem{
		Name:       parseNumber(landingWb.Total.Name),
		LeverArm:   parseNumber(fmt.Sprintf("%.4f", landingWb.Total.Arm)),
		Mass:       parseNumber(fmt.Sprintf("%.2f", landingWb.Total.Mass.Kilo())),
		MassMoment: parseNumber(fmt.Sprintf("%.2f", landingWb.Total.KGM())),
	}

	wbState.WithinLimits = wb.WithinLimits
	template.ChartUrl = fmt.Sprintf("/aquila-wb?callsign=%s&takeoff-mass=%.2f&takeoff-mass-moment=%.2f&landing-mass=%.2f&landing-mass-moment=%.2f&limits=%t", s.CallSign.String(), wb.Total.Mass, wb.Total.KGM(), landingWb.Total.Mass, landingWb.Total.KGM(), wb.WithinLimits)

	template.WeightAndBalanceTakeOff = wbState
	template.WeightAndBalanceLanding = wbLandingState
	return template
}
