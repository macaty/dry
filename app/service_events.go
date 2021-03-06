package app

import (
	"fmt"
	"strconv"

	"github.com/moncho/dry/appui"
	"github.com/moncho/dry/ui"
	"github.com/moncho/dry/ui/json"
	termbox "github.com/nsf/termbox-go"
)

type servicesScreenEventHandler struct {
	baseEventHandler
}

func (h *servicesScreenEventHandler) widget() appui.AppWidget {
	return h.dry.widgetRegistry.ServiceList
}

func (h *servicesScreenEventHandler) handle(event termbox.Event) {
	if h.forwardingEvents {
		h.eventChan <- event
		return
	}
	handled := false
	focus := true
	dry := h.dry

	switch event.Key {
	case termbox.KeyF1: // refresh
		h.dry.widgetRegistry.ServiceList.Sort()
		handled = true
	case termbox.KeyF5: // refresh
		h.dry.appmessage("Refreshing the service list")
		if err := h.widget().Unmount(); err != nil {
			h.dry.appmessage("There was an error refreshing the service list: " + err.Error())
		}
		handled = true
	case termbox.KeyCtrlR:

		rw := appui.NewPrompt("The selected service will be removed. Do you want to proceed? y/N")
		h.setForwardEvents(true)
		handled = true
		dry.widgetRegistry.add(rw)
		go func() {
			events := ui.EventSource{
				Events: h.eventChan,
				EventHandledCallback: func(e termbox.Event) error {
					return refreshScreen()
				},
			}
			rw.OnFocus(events)
			dry.widgetRegistry.remove(rw)
			confirmation, canceled := rw.Text()
			h.setForwardEvents(false)
			if canceled || (confirmation != "y" && confirmation != "Y") {
				return
			}
			removeService := func(serviceID string) error {
				err := dry.dockerDaemon.ServiceRemove(serviceID)
				refreshScreen()
				return err
			}
			if err := h.widget().OnEvent(removeService); err != nil {
				h.dry.appmessage("There was an error removing the service: " + err.Error())
			}
		}()

	case termbox.KeyCtrlS:

		rw := appui.NewPrompt("Scale service. Number of replicas?")
		h.setForwardEvents(true)
		handled = true
		dry.widgetRegistry.add(rw)
		go func() {
			events := ui.EventSource{
				Events: h.eventChan,
				EventHandledCallback: func(e termbox.Event) error {
					return refreshScreen()
				},
			}
			rw.OnFocus(events)
			dry.widgetRegistry.remove(rw)
			replicas, canceled := rw.Text()
			h.setForwardEvents(false)
			if canceled {
				return
			}
			scaleTo, err := strconv.Atoi(replicas)
			if err != nil || scaleTo < 0 {
				dry.appmessage(
					fmt.Sprintf("Cannot scale service, invalid number of replicas: %s", replicas))
				return
			}

			scaleService := func(serviceID string) error {
				err := dry.dockerDaemon.ServiceScale(serviceID, uint64(scaleTo))

				if err == nil {
					dry.appmessage(fmt.Sprintf("Service %s scaled to %d replicas", serviceID, scaleTo))
				}
				refreshScreen()
				return err
			}
			if err := h.widget().OnEvent(scaleService); err != nil {
				h.dry.appmessage("There was an error scaling the service: " + err.Error())
			}
		}()

	case termbox.KeyEnter:
		showTasks := func(serviceID string) error {
			h.screen.Cursor.Reset()
			h.dry.ShowServiceTasks(serviceID)
			return refreshScreen()
		}
		h.widget().OnEvent(showTasks)
		handled = true
	}
	switch event.Ch {
	case '%':
		handled = true
		showFilterInput(h)
	case 'i' | 'I':
		handled = true

		inspectService := func(serviceID string) error {
			service, err := h.dry.ServiceInspect(serviceID)
			if err == nil {
				go func() {
					defer func() {
						h.closeViewChan <- struct{}{}
					}()
					v, err := json.NewViewer(
						h.screen,
						appui.DryTheme,
						service)
					if err != nil {
						dry.appmessage(
							fmt.Sprintf("Error inspecting service: %s", err.Error()))
						return
					}
					v.Focus(h.eventChan)
				}()
				return nil
			}
			return err
		}
		if err := h.widget().OnEvent(inspectService); err == nil {
			focus = false
		} else {
			h.dry.appmessage("There was an error inspecting the service: " + err.Error())
		}

	case 'l':

		prompt := logsPrompt()
		h.setForwardEvents(true)
		handled = true
		dry.widgetRegistry.add(prompt)
		go func() {
			events := ui.EventSource{
				Events: h.eventChan,
				EventHandledCallback: func(e termbox.Event) error {
					return refreshScreen()
				},
			}
			prompt.OnFocus(events)
			dry.widgetRegistry.remove(prompt)
			since, canceled := prompt.Text()

			if canceled {
				h.setForwardEvents(false)
				return
			}

			showServiceLogs := func(serviceID string) error {
				logs, err := h.dry.dockerDaemon.ServiceLogs(serviceID, since)
				if err == nil {
					appui.Stream(logs, h.eventChan,
						func() {
							h.setForwardEvents(false)
							h.closeViewChan <- struct{}{}
						})
					return nil
				}
				return err
			}
			if err := h.widget().OnEvent(showServiceLogs); err != nil {
				h.dry.appmessage("There was an error showing service logs: " + err.Error())
				h.setForwardEvents(false)

			}
		}()
	}
	if !handled {
		h.baseEventHandler.handle(event)
	} else {
		h.setFocus(focus)
		if h.hasFocus() {
			refreshScreen()
		}
	}
}

func logsPrompt() *appui.Prompt {
	return appui.NewPrompt("Show logs since timestamp (e.g. 2013-01-02T13:23:37) or relative (e.g. 42m for 42 minutes) or leave empty")
}
