# Test data

`public-rackmount1/` is a subset of DMTF's public Redfish mockup of a rack-mount server, copied
unchanged from [DMTF/Redfish-Mockup-Server](https://github.com/DMTF/Redfish-Mockup-Server)
(commit 23d21f9, 2026-09-04) under the BSD 3-Clause licence in `public-rackmount1/LICENSE.md`.
Only the resources the driver reads are included. The tests serve them the way DMTF's mockup
server does: `/redfish/v1/<path>` is `<path>/index.json`.

The mockup is not self-consistent, which suits the tests: the chassis reports 374 W, its one
installed power supply reports 937.4 W input, and the power supply's energy counter is a whole
number of kWh.
