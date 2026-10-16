package redfish

import (
	"context"
	"fmt"
	"slices"
)

// ResetType is a value of ComputerSystem.Reset's ResetType parameter.
type ResetType string

const (
	On               ResetType = "On"
	GracefulShutdown ResetType = "GracefulShutdown"
	ForceOff         ResetType = "ForceOff"
)

type (
	systemRes struct {
		PowerState string
		Actions    struct {
			Reset *resetAction `json:"#ComputerSystem.Reset"`
		}
	}
	resetAction struct {
		Target     string   `json:"target"`
		Allowable  []string `json:"ResetType@Redfish.AllowableValues"`
		ActionInfo string   `json:"@Redfish.ActionInfo"`
	}
	actionInfo struct {
		Parameters []struct {
			Name            string
			AllowableValues []string
		}
	}
)

// PowerState returns the system's power state as the BMC reports it: On, Off,
// PoweringOn or PoweringOff.
func (d *Driver) PowerState(ctx context.Context, system string) (string, error) {
	var s systemRes
	if err := d.c.get(ctx, system, &s); err != nil {
		return "", err
	}
	return s.PowerState, nil
}

// Reset asks the BMC to apply a reset type to a system, one of
// Inventory.Systems. It refuses a type the BMC does not list as allowed. It
// returns when the BMC has accepted the request, not when the system has
// changed state: poll PowerState for that.
func (d *Driver) Reset(ctx context.Context, system string, t ResetType) error {
	var s systemRes
	if err := d.c.get(ctx, system, &s); err != nil {
		return err
	}
	a := s.Actions.Reset
	if a == nil || a.Target == "" {
		return fmt.Errorf("redfish: %s offers no ComputerSystem.Reset", system)
	}
	allowed := a.Allowable
	if len(allowed) == 0 && a.ActionInfo != "" {
		var info actionInfo
		if err := d.c.get(ctx, a.ActionInfo, &info); err != nil {
			return err
		}
		for _, p := range info.Parameters {
			if p.Name == "ResetType" {
				allowed = p.AllowableValues
			}
		}
	}
	if len(allowed) == 0 {
		d.quirk("%s: ComputerSystem.Reset lists no allowed reset types", system)
	} else if !slices.Contains(allowed, string(t)) {
		return fmt.Errorf("redfish: %s does not allow reset type %s (allowed: %v)", system, t, allowed)
	}
	return d.c.do(ctx, "POST", a.Target, map[string]string{"ResetType": string(t)}, nil)
}
