module gnss-spoofer

go 1.25.3

require (
	github.com/chzyer/readline v1.5.1
	go.bug.st/serial v1.6.4
	serialport v0.0.0
)

require (
	github.com/creack/goselect v0.1.2 // indirect
	github.com/manifoldco/promptui v0.9.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
)

replace serialport => ../serialport
