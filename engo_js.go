//+build netgo

package engo

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"image/color"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"strconv"
	"time"

	"engo.io/gl"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/gopherjs/gopherjs/js"
	"honnef.co/go/js/dom"
	"honnef.co/go/js/xhr"
)

var (
	Gl                        *gl.Context
	gameWidth, gameHeight     float32
	windowWidth, windowHeight float32
)

func init() {
	rafPolyfill()
}

//var canvas *js.Object
var document = dom.GetWindow().Document().(dom.HTMLDocument)

func CreateWindow(title string, width, height int, fullscreen bool) {

	canvas := document.CreateElement("canvas").(*dom.HTMLCanvasElement)

	devicePixelRatio := js.Global.Get("devicePixelRatio").Float()
	canvas.Width = int(float64(width)*devicePixelRatio + 0.5)   // Nearest non-negative int.
	canvas.Height = int(float64(height)*devicePixelRatio + 0.5) // Nearest non-negative int.
	canvas.Style().SetProperty("width", fmt.Sprintf("%vpx", width), "")
	canvas.Style().SetProperty("height", fmt.Sprintf("%vpx", height), "")

	if js.Global.Get("document").Get("body") == nil {
		body := js.Global.Get("document").Call("createElement", "body")
		js.Global.Get("document").Set("body", body)
		log.Println("Creating body, since it doesn't exist.")
	}
	document.Body().Style().SetProperty("margin", "0", "")
	document.Body().AppendChild(canvas)

	document.SetTitle(title)

	var err error

	Gl, err = gl.NewContext(canvas.Underlying(), nil) // TODO: we can add arguments here
	if err != nil {
		log.Println("Could not create context:", err)
		return
	}
	Gl.Viewport(0, 0, width, height)

	// DEBUG: Add framebuffer information div.
	if false {
		//canvas.Height -= 30
		text := document.CreateElement("div")
		textContent := fmt.Sprintf("%v %v (%v) @%v", dom.GetWindow().InnerWidth(), canvas.Width, float64(width)*devicePixelRatio, devicePixelRatio)
		text.SetTextContent(textContent)
		document.Body().AppendChild(text)
	}
	gameWidth = float32(width)
	gameHeight = float32(height)
	windowWidth = WindowWidth()
	windowHeight = WindowHeight()
	w := dom.GetWindow()
	w.AddEventListener("keypress", false, func(ev dom.Event) {
		// TODO: Not sure what to do here, come back
		//ke := ev.(*dom.KeyboardEvent)
		//responser.Type(rune(keyStates[Key(ke.KeyCode)]))
	})
	w.AddEventListener("keydown", false, func(ev dom.Event) {
		ke := ev.(*dom.KeyboardEvent)
		keyStates[Key(ke.KeyCode)] = true
	})
	w.AddEventListener("keyup", false, func(ev dom.Event) {
		ke := ev.(*dom.KeyboardEvent)
		keyStates[Key(ke.KeyCode)] = false
	})

	Files = NewLoader()
	WorldBounds.Max = Point{GameWidth(), GameHeight()}
}

func SetBackground(c color.Color) {
	if !headless {
		r, g, b, a := c.RGBA()
		Gl.ClearColor(float32(r), float32(g), float32(b), float32(a))
		Gl.Clear(Gl.COLOR_BUFFER_BIT)
	}
}

func DestroyWindow() {
	// TODO: anything to do here?
}

func GameWidth() float32 {
	return gameWidth
}

func GameHeight() float32 {
	return gameHeight
}

func CursorPos() (x, y float64) {
	return 0.0, 0.0
}

func WindowSize() (w, h int) {
	w = int(WindowWidth())
	h = int(WindowHeight())
	return
}

func WindowWidth() float32 {
	return float32(dom.GetWindow().InnerWidth())
}

func WindowHeight() float32 {
	return float32(dom.GetWindow().InnerHeight())
}

func toPx(n int) string {
	return strconv.FormatInt(int64(n), 10) + "px"
}

func rafPolyfill() {
	window := js.Global
	vendors := []string{"ms", "moz", "webkit", "o"}
	if window.Get("requestAnimationFrame") == nil {
		for i := 0; i < len(vendors) && window.Get("requestAnimationFrame") == nil; i++ {
			vendor := vendors[i]
			window.Set("requestAnimationFrame", window.Get(vendor+"RequestAnimationFrame"))
			window.Set("cancelAnimationFrame", window.Get(vendor+"CancelAnimationFrame"))
			if window.Get("cancelAnimationFrame") == nil {
				window.Set("cancelAnimationFrame", window.Get(vendor+"CancelRequestAnimationFrame"))
			}
		}
	}

	lastTime := 0.0
	if window.Get("requestAnimationFrame") == nil {
		window.Set("requestAnimationFrame", func(callback func(float32)) int {
			currTime := js.Global.Get("Date").New().Call("getTime").Float()
			timeToCall := math.Max(0, 16-(currTime-lastTime))
			id := window.Call("setTimeout", func() { callback(float32(currTime + timeToCall)) }, timeToCall)
			lastTime = currTime + timeToCall
			return id.Int()
		})
	}

	if window.Get("cancelAnimationFrame") == nil {
		window.Set("cancelAnimationFrame", func(id int) {
			js.Global.Get("clearTimeout").Invoke(id)
		})
	}
}

func RunIteration() {
	keysUpdate()
	currentWorld.Update(Time.Delta())
	Time.Tick()
	// TODO: this may not work, and sky-rocket the FPS
	//  requestAnimationFrame(func(dt float32) {
	// 	currentWorld.Update(Time.Delta())
	// 	keysUpdate()
	// 	if !headless {
	// 		// TODO: does this require !headless?
	// 		Mouse.ScrollX, Mouse.ScrollY = 0, 0
	// 	}
	// 	Time.Tick()
	// })
}

func requestAnimationFrame(callback func(float32)) int {
	//return dom.GetWindow().RequestAnimationFrame(callback)
	return js.Global.Call("requestAnimationFrame", callback).Int()
}

func cancelAnimationFrame(id int) {
	dom.GetWindow().CancelAnimationFrame(id)
}

func RunPreparation() {
	keyStates = make(map[Key]bool)
	Time = NewClock()

	dom.GetWindow().AddEventListener("onbeforeunload", false, func(e dom.Event) {
		dom.GetWindow().Alert("You're closing")
	})
}

func runLoop(defaultScene Scene, headless bool) {
	SetScene(defaultScene, false)
	RunPreparation()
	ticker := time.NewTicker(time.Duration(int(time.Second) / fpsLimit))
Outer:
	for {
		select {
		case <-ticker.C:
			if closeGame {
				break Outer
			}
			RunIteration()
		case <-resetLoopTicker:
			ticker.Stop()
			ticker = time.NewTicker(time.Duration(int(time.Second) / fpsLimit))
		}
	}
	ticker.Stop()
}

func loadImage(r Resource) (Image, error) {
	ch := make(chan error, 1)

	img := js.Global.Get("Image").New()
	img.Call("addEventListener", "load", func(*js.Object) {
		go func() { ch <- nil }()
	}, false)
	img.Call("addEventListener", "error", func(o *js.Object) {
		go func() { ch <- &js.Error{Object: o} }()
	}, false)
	img.Set("src", r.url+"?"+strconv.FormatInt(rand.Int63(), 10))

	err := <-ch
	if err != nil {
		return nil, err
	}
	log.Println()

	return &ImageObject{img}, nil
}

func loadJSON(r Resource) (string, error) {
	req := xhr.NewRequest("GET", r.url)
	err := req.Send("")
	if err != nil {
		return "", err
	}
	return req.Response.String(), nil
	// ch := make(chan error, 1)

	// req := js.Global.Get("XMLHttpRequest").New()
	// req.Call("open", "GET", r.url, true)
	// req.Call("addEventListener", "load", func(*js.Object) {
	// 	go func() { ch <- nil }()
	// }, false)
	// req.Call("addEventListener", "error", func(o *js.Object) {
	// 	go func() { ch <- &js.Error{Object: o} }()
	// }, false)
	// req.Call("send", nil)

	// err := <-ch
	// if err != nil {
	// 	return "", err
	// }

	// return req.Get("responseText").Str(), nil
}

func loadFont(r Resource) (*truetype.Font, error) {
	req := xhr.NewRequest("GET", r.url+"_js")
	err := req.Send("")
	if err != nil {
		return &truetype.Font{}, err
	}
	fontDataEncoded := bytes.NewBuffer([]byte(req.Response.String()))
	fontDataCompressed := base64.NewDecoder(base64.StdEncoding, fontDataEncoded)
	fontDataTtf, err := gzip.NewReader(fontDataCompressed)
	if err != nil {
		return nil, err
	}
	var ttfBytes []byte
	ttfBytes, err = ioutil.ReadAll(fontDataTtf)
	if err != nil {
		return nil, err
	}
	log.Println(ttfBytes)
	return freetype.ParseFont(ttfBytes)
}

type ImageObject struct {
	data *js.Object
}

func NewImageObject(img *js.Object) *ImageObject {
	return &ImageObject{img}
}

func (i *ImageObject) Data() interface{} {
	return i.data
}

func (i *ImageObject) Width() int {
	return i.data.Get("width").Int()
}

func (i *ImageObject) Height() int {
	return i.data.Get("height").Int()
}