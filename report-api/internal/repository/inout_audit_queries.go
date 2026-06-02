package repository

import (
	"fmt"

	"github.com/google/uuid"
)

const auditDateRange = "event_time >= toDateTime64(?, 3, 'UTC') AND event_time < addDays(toDateTime64(?, 3, 'UTC'), 1)"

const (
	selectCountFromInoutEventsWhere = "SELECT COUNT(*) FROM inout_events WHERE "
	whereSQL                        = " WHERE "
)

type auditQuerySet struct {
	count     string
	selectSQL string
}

// auditQueries maps a filter bitmask to fully static SQL (no runtime string formatting).
// Bit 1: employee, bit 2: door, bit 4: status, bit 8: org units.
var auditQueries = map[uint8]auditQuerySet{
	0b0000: {
		count:     selectCountFromInoutEventsWhere + auditDateRange,
		selectSQL: auditSelectCols + whereSQL + auditDateRange + auditOrderLimit,
	},
	0b0001: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND employee_id = ?",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND employee_id = ?" + auditOrderLimit,
	},
	0b0010: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND door_id = ?",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND door_id = ?" + auditOrderLimit,
	},
	0b0011: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND employee_id = ? AND door_id = ?",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND employee_id = ? AND door_id = ?" + auditOrderLimit,
	},
	0b0100: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND status = ?",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND status = ?" + auditOrderLimit,
	},
	0b0101: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND employee_id = ? AND status = ?",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND employee_id = ? AND status = ?" + auditOrderLimit,
	},
	0b0110: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND door_id = ? AND status = ?",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND door_id = ? AND status = ?" + auditOrderLimit,
	},
	0b0111: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND employee_id = ? AND door_id = ? AND status = ?",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND employee_id = ? AND door_id = ? AND status = ?" + auditOrderLimit,
	},
	0b1000: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND org_unit_id IN (?)",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND org_unit_id IN (?)" + auditOrderLimit,
	},
	0b1001: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND employee_id = ? AND org_unit_id IN (?)",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND employee_id = ? AND org_unit_id IN (?)" + auditOrderLimit,
	},
	0b1010: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND door_id = ? AND org_unit_id IN (?)",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND door_id = ? AND org_unit_id IN (?)" + auditOrderLimit,
	},
	0b1011: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND employee_id = ? AND door_id = ? AND org_unit_id IN (?)",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND employee_id = ? AND door_id = ? AND org_unit_id IN (?)" + auditOrderLimit,
	},
	0b1100: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND status = ? AND org_unit_id IN (?)",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND status = ? AND org_unit_id IN (?)" + auditOrderLimit,
	},
	0b1101: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND employee_id = ? AND status = ? AND org_unit_id IN (?)",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND employee_id = ? AND status = ? AND org_unit_id IN (?)" + auditOrderLimit,
	},
	0b1110: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND door_id = ? AND status = ? AND org_unit_id IN (?)",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND door_id = ? AND status = ? AND org_unit_id IN (?)" + auditOrderLimit,
	},
	0b1111: {
		count:     selectCountFromInoutEventsWhere + auditDateRange + " AND employee_id = ? AND door_id = ? AND status = ? AND org_unit_id IN (?)",
		selectSQL: auditSelectCols + whereSQL + auditDateRange + " AND employee_id = ? AND door_id = ? AND status = ? AND org_unit_id IN (?)" + auditOrderLimit,
	},
}

const auditSelectCols = `
		SELECT id, employee_id, door_id, direction, event_time,
		       status, reason, COALESCE(card_uid,''), COALESCE(source_ip,'')
		FROM inout_events`

const auditOrderLimit = `
		ORDER BY event_time DESC
		LIMIT ? OFFSET ?`

func buildAuditQuery(f AuditFilter) (auditQuerySet, []interface{}, error) {
	var mask uint8
	args := []interface{}{f.StartDate, f.EndDate}

	var err error
	mask, args, err = appendEmployeeFilter(f.EmployeeID, mask, args)
	if err != nil {
		return auditQuerySet{}, nil, err
	}
	mask, args, err = appendDoorFilter(f.DoorID, mask, args)
	if err != nil {
		return auditQuerySet{}, nil, err
	}
	mask, args, err = appendStatusFilter(f.Status, mask, args)
	if err != nil {
		return auditQuerySet{}, nil, err
	}
	mask, args = appendOrgUnitsFilter(f.OrgUnitIDs, mask, args)

	q, ok := auditQueries[mask]
	if !ok {
		return auditQuerySet{}, nil, fmt.Errorf("unsupported audit filter combination")
	}
	return q, args, nil
}

func appendEmployeeFilter(employeeID string, mask uint8, args []interface{}) (uint8, []interface{}, error) {
	if employeeID != "" {
		empUUID, err := uuid.Parse(employeeID)
		if err != nil {
			return mask, nil, fmt.Errorf("invalid employee id: %w", err)
		}
		mask |= 0b0001
		args = append(args, empUUID)
	}
	return mask, args, nil
}

func appendDoorFilter(doorID string, mask uint8, args []interface{}) (uint8, []interface{}, error) {
	if doorID != "" {
		doorUUID, err := uuid.Parse(doorID)
		if err != nil {
			return mask, nil, fmt.Errorf("invalid door id: %w", err)
		}
		mask |= 0b0010
		args = append(args, doorUUID)
	}
	return mask, args, nil
}

func appendStatusFilter(status string, mask uint8, args []interface{}) (uint8, []interface{}, error) {
	if status != "" {
		if status != "ALLOW" && status != "DENY" {
			return mask, nil, fmt.Errorf("invalid status: %s", status)
		}
		mask |= 0b0100
		args = append(args, status)
	}
	return mask, args, nil
}

func appendOrgUnitsFilter(orgUnitIDs []string, mask uint8, args []interface{}) (uint8, []interface{}) {
	if len(orgUnitIDs) > 0 {
		var orgUUIDs []uuid.UUID
		for _, id := range orgUnitIDs {
			u, err := uuid.Parse(id)
			if err == nil {
				orgUUIDs = append(orgUUIDs, u)
			}
		}
		mask |= 0b1000
		args = append(args, orgUUIDs)
	}
	return mask, args
}
