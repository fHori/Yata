package defs

import "testing"

func TestApprovalStatus(t *testing.T) {
	cases := []struct {
		name string
		a    *DefApproval
		want string
	}{
		{"absent block", nil, ApprovalUnknown},
		{"empty block (the bundled-def default)", &DefApproval{}, ApprovalUnknown},
		{"name without date", &DefApproval{Name: "SysOp"}, ApprovalUnknown},
		{"date without name", &DefApproval{Date: "2026-07-01"}, ApprovalUnknown},
		{"name + date = approved", &DefApproval{Name: "SysOp", Date: "2026-07-01"}, ApprovalApproved},
		{"whitespace name is not filled", &DefApproval{Name: "  ", Date: "2026-07-01"}, ApprovalUnknown},
		{"informal overrides filled fields", &DefApproval{Name: "Mod", Date: "2026-07-01", Status: ApprovalInformal}, ApprovalInformal},
		{"pending", &DefApproval{Status: ApprovalPending}, ApprovalPending},
		{"unrecognised status falls back to derivation", &DefApproval{Status: "yes", Name: "SysOp", Date: "2026-07-01"}, ApprovalApproved},
	}
	for _, c := range cases {
		d := TrackerDef{ApprovedBy: c.a}
		if got := d.ApprovalStatus(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
