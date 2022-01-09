package util

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	SND_SYNC      uint = 0x0000 /* play synchronously (default) */
	SND_ASYNC     uint = 0x0001 /* play asynchronously */
	SND_NODEFAULT uint = 0x0002 /* silence (!default) if sound not found */
	SND_MEMORY    uint = 0x0004 /* pszSound points to a memory file */
	SND_LOOP      uint = 0x0008 /* loop the sound until next sndPlaySound */
	SND_NOSTOP    uint = 0x0010 /* don't stop any currently playing sound */

	SND_NOWAIT      uint = 0x00002000 /* don't wait if the driver is busy */
	SND_ALIAS       uint = 0x00010000 /* name is a registry alias */
	SND_ALIAS_ID    uint = 0x00110000 /* alias is a predefined ID */
	SND_FILENAME    uint = 0x00020000 /* name is file name */
	SND_RESOURCE    uint = 0x00040004 /* name is resource name or atom */
	SND_PURGE       uint = 0x0040     /* purge non-static events for task */
	SND_APPLICATION uint = 0x0080     /* look for application specific association */
	SND_SENTRY      uint = 0x00080000 /* Generate a SoundSentry event with this sound */
	SND_RING        uint = 0x00100000 /* Treat this as a "ring" from a communications app - don't duck me */
	SND_SYSTEM      uint = 0x00200000 /* Treat this as a system sound */

	SND_ALIAS_START uint = 0 /* alias base */
)

var (
	mmsystem = syscall.MustLoadDLL("winmm.dll")

	playSound     = mmsystem.MustFindProc("PlaySound")
	sndPlaySoundA = mmsystem.MustFindProc("sndPlaySoundA")
	sndPlaySoundW = mmsystem.MustFindProc("sndPlaySoundW")
)

// PlaySound play sound in Windows
func PlaySound(sound string, hmod int, flags uint) {
	s16, _ := syscall.UTF16PtrFromString(sound)
	playSound.Call(uintptr(unsafe.Pointer(s16)), uintptr(hmod), uintptr(flags))
}

// SndPlaySoundA play sound file in Windows
func SndPlaySoundA(sound string, flags uint) {
	b := append([]byte(sound), 0)
	sndPlaySoundA.Call(uintptr(unsafe.Pointer(&b[0])), uintptr(flags))
}

// SndPlaySoundW play sound file in Windows
func SndPlaySoundW(sound string, flags uint) {
	s16, _ := syscall.UTF16PtrFromString(sound)
	sndPlaySoundW.Call(uintptr(unsafe.Pointer(s16)), uintptr(flags))
}

func init() {
	fmt.Println("initializing ...")
}
