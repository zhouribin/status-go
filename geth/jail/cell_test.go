package jail_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/robertkrimen/otto"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/status-im/status-go/geth/jail"
	"github.com/status-im/status-go/static"
)

const testChatID = "testChat"

var baseStatusJSCode = string(static.MustAsset("testdata/jail/status.js"))

func TestJailTimeout(t *testing.T) {
	Convey("Jail should handle setTimeout correctly", t, func() {
		cell, err := jail.New(nil).NewCell(testChatID)
		So(err, ShouldBeNil)
		So(cell, ShouldNotBeNil)
		Reset(func() {
			cell.Stop()
		})

		Convey("setTimeout single function", func() {
			_, err = cell.Run(`
			var timerCounts = 0;
			setTimeout(function(n){
				if (Date.now() - n < 50) {
					throw new Error("Timed out");
				}

				timerCounts++;
			}, 50, Date.now());
			`)
			So(err, ShouldBeNil)

			// wait at least 10x longer to decrease probability
			// of false negatives as we using real clock here
			time.Sleep(300 * time.Millisecond)

			value, err := cell.Get("timerCounts")
			So(err, ShouldBeNil)
			So(value.IsNumber(), ShouldBeTrue)
			So(value.String(), ShouldEqual, "1")
		})
		Convey("multiple setTimeout shouldn't race", func() {
			items := make(chan struct{})

			err = cell.Set("__captureResponse", func() otto.Value {
				go func() { items <- struct{}{} }()
				return otto.UndefinedValue()
			})
			So(err, ShouldBeNil)

			_, err = cell.Run(`
			function callTimeoutFunc(){
				return setTimeout(function(){
					__captureResponse();
				}, 1000);
			}
			`)
			So(err, ShouldBeNil)

			N := 25
			for i := 0; i < N; i++ {
				_, err = cell.Call("callTimeoutFunc", nil)
				So(err, ShouldBeNil)
			}
			for i := 0; i < N; i++ {
				select {
				case <-items:
				case <-time.After(5 * time.Second):
					t.Fatal("test timed out")
				}
			}
		})
		Convey("setTimeout should be cancelled properly", func() {
			var count int
			err = cell.Set("__captureResponse", func(val string) otto.Value {
				count++
				return otto.UndefinedValue()
			})
			So(err, ShouldBeNil)

			_, err = cell.Run(`
			function callTimeoutFunc(val, delay){
				return setTimeout(function(){
					__captureResponse(val);
				}, delay);
			}
			`)
			So(err, ShouldBeNil)

			// Run 5 timeout tasks to be executed in: 1, 2, 3, 4 and 5 secs
			for i := 1; i <= 5; i++ {
				_, err = cell.Call("callTimeoutFunc", nil, "value", i*100)
				So(err, ShouldBeNil)
			}

			// Wait 1.5 second (so only one task executed) so far
			// and stop the cell (event loop should die)
			time.Sleep(150 * time.Millisecond)
			cell.Stop()

			// check that only 1 task has increased counter
			So(count, ShouldEqual, 1)

			// wait 2 seconds more (so at least two more tasks would
			// have been executed if event loop is still running)
			<-time.After(200 * time.Millisecond)

			// check that counter hasn't increased
			So(count, ShouldEqual, 1)
		})
	})
}

func TestJailFetch(t *testing.T) {
	Convey("Jail should handle FetchAPI correctly", t, func() {
		cell, err := jail.New(nil).NewCell(testChatID)
		So(err, ShouldBeNil)
		So(cell, ShouldNotBeNil)

		body := `{"key": "value"}`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Type", "application/json")
			w.Write([]byte(body))
		}))

		Reset(func() {
			cell.Stop()
			server.Close()
		})

		dataCh := make(chan otto.Value, 1)
		errCh := make(chan otto.Value, 1)

		err = cell.Set("__captureSuccess", func(res otto.Value) { dataCh <- res })
		So(err, ShouldBeNil)
		err = cell.Set("__captureError", func(res otto.Value) { errCh <- res })
		So(err, ShouldBeNil)

		Convey("Fetching valid URL should work", func() {
			_, err = cell.Run(`fetch('` + server.URL + `').then(function(r) {
				return r.text()
			}).then(function(data) {
				__captureSuccess(data)
			}).catch(function (e) {
				__captureError(e)
			})`)
			So(err, ShouldBeNil)

			select {
			case data := <-dataCh:
				So(data.IsString(), ShouldBeTrue)
				So(data.String(), ShouldEqual, body)
			case err := <-errCh:
				t.Fatalf("request failed: %v", err)
			case <-time.After(1 * time.Second):
				t.Fatal("test timed out")
			}
		})
		Convey("Fetching invalid URL should provoke error", func() {
			_, err = cell.Run(`fetch('http://👽/nonexistent').then(function(r) {
				return r.text()
			}).then(function(data) {
				__captureSuccess(data)
			}).catch(function (e) {
				__captureError(e)
			})`)
			So(err, ShouldBeNil)

			select {
			case data := <-dataCh:
				t.Fatalf("request should have failed, but returned: %v", data)
			case e := <-errCh:
				So(e.IsObject(), ShouldBeTrue)
				name, err := e.Object().Get("name")
				So(err, ShouldBeNil)
				So(name.String(), ShouldEqual, "Error")
				msg, err := e.Object().Get("message")
				So(err, ShouldBeNil)
				So(msg.String(), ShouldNotBeEmpty)
			case <-time.After(1 * time.Second):
				t.Fatal("test timed out")
			}
		})
		Convey("Fetching concurrently shouldn't race", func() {
			_, err := cell.Run(`fetch('` + server.URL + `').then(function(r) {
				return r.text()
			}).then(function(data) {
				__captureSuccess(data)
			}).catch(function (e) {
				__captureError(e)
			})`)
			So(err, ShouldBeNil)

			_, err = cell.Run(`fetch('http://👽/nonexistent').then(function(r) {
				return r.text()
			}).then(function(data) {
				__captureSuccess(data)
			}).catch(function (e) {
				__captureError(e)
			})`)
			So(err, ShouldBeNil)

			for i := 0; i < 2; i++ {
				select {
				case data := <-dataCh:
					So(data.IsString(), ShouldBeTrue)
					So(data.String(), ShouldEqual, body)
				case e := <-errCh:
					So(e.IsObject(), ShouldBeTrue)
					name, err := e.Object().Get("name")
					So(err, ShouldBeNil)
					So(name.String(), ShouldEqual, "Error")
					msg, err := e.Object().Get("message")
					So(err, ShouldBeNil)
					So(msg.String(), ShouldNotBeEmpty)
				case <-time.After(1 * time.Second):
					t.Fatal("test timed out")
					return
				}
			}
		})
	})
}
