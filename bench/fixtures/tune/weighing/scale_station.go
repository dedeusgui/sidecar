package weighing

// MaxWeighingSlots limits how many trucks may be weighed at the same time.
const MaxWeighingSlots = 4

// ScaleStation runs the platform scale of the terminal.
type ScaleStation struct {
	occupied int
}

// ConfirmWeight records the weight of the truck standing on the scale.
func (s *ScaleStation) ConfirmWeight(plate string, kilos int) bool {
	return plate != "" && kilos > 0 && s.occupied <= MaxWeighingSlots
}

// ReleaseTruck lets a weighed truck leave the scale area.
func (s *ScaleStation) ReleaseTruck(plate string) error {
	s.occupied--
	return nil
}
