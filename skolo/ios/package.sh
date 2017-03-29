#!/bin/bash

set -x -e

APPNAME=skia-imobiledevice
SERVICE_FILE="path-to-service-file.service"

# Builds and uploads a debian package for skiacorrectness.
SYSTEMD="usbmuxd.service"
DESCRIPTION="Latest versions of libimobiledevice and related tools."
IN_DIR="$(pwd)/out"
OUT_DIR="usr/local"

# Make sure we restart udev rules and bypass the upload.
UDEV_RELOAD=True
BYPASS_UPLOAD=True

# Copy files into the right locations in ${ROOT}.
copy_release_files()
{
INSTALL="sudo install -D --verbose --backup=none --group=root --owner=root"
INSTALL_DIR="sudo install -d --verbose --backup=none --group=root --owner=root"

${INSTALL_DIR} --mode=755                                                    ${ROOT}/${OUT_DIR}/sbin
${INSTALL}     --mode=755 -T ${IN_DIR}/sbin/usbmuxd                          ${ROOT}/${OUT_DIR}/sbin/usbmuxd

${INSTALL_DIR} --mode=755                                                    ${ROOT}/${OUT_DIR}/bin
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/idevicename                       ${ROOT}/${OUT_DIR}/bin/idevicename
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/ifuse                             ${ROOT}/${OUT_DIR}/bin/ifuse
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/ideviceprovision                  ${ROOT}/${OUT_DIR}/bin/ideviceprovision
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/ideviceinstaller                  ${ROOT}/${OUT_DIR}/bin/ideviceinstaller
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/idevicesyslog                     ${ROOT}/${OUT_DIR}/bin/idevicesyslog
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/idevicebackup                     ${ROOT}/${OUT_DIR}/bin/idevicebackup
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/plistutil                         ${ROOT}/${OUT_DIR}/bin/plistutil
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/idevicebackup2                    ${ROOT}/${OUT_DIR}/bin/idevicebackup2
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/idevicepair                       ${ROOT}/${OUT_DIR}/bin/idevicepair
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/ideviceimagemounter               ${ROOT}/${OUT_DIR}/bin/ideviceimagemounter
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/idevicedebugserverproxy           ${ROOT}/${OUT_DIR}/bin/idevicedebugserverproxy
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/idevicenotificationproxy          ${ROOT}/${OUT_DIR}/bin/idevicenotificationproxy
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/idevice_id                        ${ROOT}/${OUT_DIR}/bin/idevice_id
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/idevicedebug                      ${ROOT}/${OUT_DIR}/bin/idevicedebug
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/idevicediagnostics                ${ROOT}/${OUT_DIR}/bin/idevicediagnostics
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/idevicedate                       ${ROOT}/${OUT_DIR}/bin/idevicedate
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/ideviceinfo                       ${ROOT}/${OUT_DIR}/bin/ideviceinfo
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/idevicecrashreport                ${ROOT}/${OUT_DIR}/bin/idevicecrashreport
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/idevicescreenshot                 ${ROOT}/${OUT_DIR}/bin/idevicescreenshot
${INSTALL}     --mode=755 -T ${IN_DIR}/bin/ideviceenterrecovery              ${ROOT}/${OUT_DIR}/bin/ideviceenterrecovery

${INSTALL_DIR} --mode=755                                                    ${ROOT}/${OUT_DIR}/lib
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libplist++.so.3.0.0               ${ROOT}/${OUT_DIR}/lib/libplist++.so.3.0.0
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libplist++.so.3                   ${ROOT}/${OUT_DIR}/lib/libplist++.so.3
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libimobiledevice.so.6             ${ROOT}/${OUT_DIR}/lib/libimobiledevice.so.6
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libplist.la                       ${ROOT}/${OUT_DIR}/lib/libplist.la
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libimobiledevice.a                ${ROOT}/${OUT_DIR}/lib/libimobiledevice.a
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libimobiledevice.so               ${ROOT}/${OUT_DIR}/lib/libimobiledevice.so

${INSTALL_DIR} --mode=755                                                    ${ROOT}/${OUT_DIR}/lib/pkgconfig
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/pkgconfig/libimobiledevice-1.0.pc ${ROOT}/${OUT_DIR}/lib/pkgconfig/libimobiledevice-1.0.pc
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/pkgconfig/libplist.pc             ${ROOT}/${OUT_DIR}/lib/pkgconfig/libplist.pc
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/pkgconfig/libplist++.pc           ${ROOT}/${OUT_DIR}/lib/pkgconfig/libplist++.pc
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libplist++.la                     ${ROOT}/${OUT_DIR}/lib/libplist++.la
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libplist.so                       ${ROOT}/${OUT_DIR}/lib/libplist.so
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libimobiledevice.so.6.0.0         ${ROOT}/${OUT_DIR}/lib/libimobiledevice.so.6.0.0
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libimobiledevice.la               ${ROOT}/${OUT_DIR}/lib/libimobiledevice.la
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libplist++.so                     ${ROOT}/${OUT_DIR}/lib/libplist++.so
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libplist.so.3                     ${ROOT}/${OUT_DIR}/lib/libplist.so.3
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libplist++.a                      ${ROOT}/${OUT_DIR}/lib/libplist++.a
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libplist.a                        ${ROOT}/${OUT_DIR}/lib/libplist.a
${INSTALL}     --mode=644 -T ${IN_DIR}/lib/libplist.so.3.0.0                 ${ROOT}/${OUT_DIR}/lib/libplist.so.3.0.0
${INSTALL}     --mode=755 -T ${IN_DIR}/udev-rules/39-usbmuxd.rules           ${ROOT}/etc/udev/rules.d/39-usbmuxd.rules
${INSTALL}     --mode=755 -T ${IN_DIR}/systemd/usbmuxd.service               ${ROOT}/etc/systemd/system/usbmuxd.service
}

source ../../bash/release.sh