# UniFi Port Profile Switcher

A small Go CLI that flips a UniFi switch port between named profile presets, by
talking to a self-hosted UniFi OS controller's internal REST API.

Built to solve a specific problem: a docking station shared between a personal
and a work laptop. Because the dock presents one MAC, the controller can't
auto-assign different VLANs — but a one-shot CLI can.

![Supports aarch64 Architecture][aarch64-shield]
![Supports amd64 Architecture][amd64-shield]

[aarch64-shield]: https://img.shields.io/badge/aarch64-yes-green.svg
[amd64-shield]: https://img.shields.io/badge/amd64-yes-green.svg
