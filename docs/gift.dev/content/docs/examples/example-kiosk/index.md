---
title: Kiosk
weight: 8
---

This example is a gift program as the operating system of its device, built on package
[`kiosk`](https://pkg.go.dev/github.com/worldiety/gift/kiosk): a counter of finished workpieces for a workbench,
and the settings of the device after those of iOS and iPadOS. The gear in the top right corner of the counter
opens them: Wi-Fi, Ethernet with fixed addresses, the display, a passcode, General with software updates, storage,
date and time and the startup, and a page of the program itself, which sets the goal of a shift.

{{< gallery-shot alt="The settings of a simulated Raspberry Pi called Werkbank: on the left a list with the device, Wi-Fi, Ethernet, General, Display, Passcode and Counter, on the right the Wi-Fi page with the joined network Werkstatt and four more networks with their signal strength" >}}

The window above has the size of a 7 inch panel, where the list stands beside the page, as on an iPad. On a
5 inch panel the settings take the layout of an iPhone: the list, and each page pushed onto it.

{{< gallery-shot prefix="compact" alt="The same settings on a 5 inch panel held upright: the list alone, under a bar with the title Settings and a Done button, each row with a chevron" >}}

## Run it

From a checkout of the [repository](https://github.com/worldiety/gift):

```bash
go run ./cmd/example-kiosk -simulate                                 # a simulated Raspberry Pi, on any machine
go run ./cmd/example-kiosk -simulate -compact -width 360 -height 640 # as on a 5 inch panel, upright
go run ./cmd/example-kiosk -simulate -pin 2580                       # with a passcode set by the program
go run ./cmd/example-kiosk                                           # the system it runs on
```

With `-simulate` the settings show a Raspberry Pi 5 that only pretends: networks to join, updates to install and a
startup to switch, with nothing changed on your machine. Without it they show the system the program runs on:
macOS read only, and the desktop of Raspberry Pi OS or Ubuntu without root, where **General → Startup → Install as
Kiosk** makes the program the kiosk of the device, as
[No install scripts](../../concepts/kiosks/#no-install-scripts) describes.

`-compact` and `-regular` fix the layout; without them the width of the window decides, at 680 logical pixels.
`-width` and `-height` set the size of the window. The settings keep what is set in them, a passcode for instance,
in the directory `werkbank` of the user's configuration directory; **General → About** shows where.

## What to try

- Tap a network in **Wi-Fi**. *Halle 2* asks for a password, and the simulation takes any of eight characters or
  more; *Gast* is open, and *Lager* is known and joins without one.
- Open the interface in **Ethernet** and set its IPv4 address to *Manual*, with an address, a subnet mask and a
  router, as in the network of a building's automation. *Apply* asks whether to keep the settings, and they come
  back by themselves after a minute unless you do.
- **General → Software Update** lists the packages of a simulated upgrade and installs them with their progress.
- **General → Startup** switches what the device starts into, the kiosk or the desktop.
- Set a passcode in **Passcode** and choose what it protects. *Werkbank* is the page of the program, **Counter**,
  whose stepper sets the goal the counter shows. Four wrong attempts make it wait a minute.
- **Display** switches between the light and the dark appearance.

## Code

{{< example-code >}}
