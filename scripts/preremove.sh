#!/bin/sh
if systemctl is-active --quiet grid; then
    systemctl stop grid
fi
if systemctl is-active --quiet gridw; then
    systemctl stop gridw
fi
