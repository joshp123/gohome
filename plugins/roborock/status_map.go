package roborock

var stateNames = map[int]string{
	0:    "unknown",
	1:    "starting",
	2:    "charger_disconnected",
	3:    "idle",
	4:    "remote_control_active",
	5:    "cleaning",
	6:    "returning_home",
	7:    "manual_mode",
	8:    "charging",
	9:    "charging_problem",
	10:   "paused",
	11:   "spot_cleaning",
	12:   "error",
	13:   "shutting_down",
	14:   "updating",
	15:   "docking",
	16:   "going_to_target",
	17:   "zoned_cleaning",
	18:   "segment_cleaning",
	22:   "emptying_the_bin",
	23:   "washing_the_mop",
	25:   "washing_the_mop_2",
	26:   "going_to_wash_the_mop",
	28:   "in_call",
	29:   "mapping",
	30:   "egg_attack",
	32:   "patrol",
	33:   "attaching_the_mop",
	34:   "detaching_the_mop",
	100:  "charging_complete",
	101:  "device_offline",
	103:  "locked",
	202:  "air_drying_stopping",
	6301: "robot_status_mopping",
	6302: "clean_mop_cleaning",
	6303: "clean_mop_mopping",
	6304: "segment_mopping",
	6305: "segment_clean_mop_cleaning",
	6306: "segment_clean_mop_mopping",
	6307: "zoned_mopping",
	6308: "zoned_clean_mop_cleaning",
	6309: "zoned_clean_mop_mopping",
	6310: "back_to_dock_washing_duster",
}

func stateName(code int) string {
	if name, ok := stateNames[code]; ok {
		return name
	}
	return "unknown"
}
