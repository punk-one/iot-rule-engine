package main

import (
	"fmt"
	"io"
	"os"
	"time"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)
func main(){
	if len(os.Args)!=2{panic("topic required")}
	opts:=mqtt.NewClientOptions().AddBroker(os.Getenv("TEST_MQTT_URL")).SetClientID(fmt.Sprintf("punkiot-test-%d",time.Now().UnixNano())).SetConnectTimeout(5*time.Second)
	opts.SetUsername(os.Getenv("TEST_MQTT_USERNAME"));opts.SetPassword(os.Getenv("TEST_MQTT_PASSWORD"))
	client:=mqtt.NewClient(opts);token:=client.Connect();if !token.WaitTimeout(10*time.Second)||token.Error()!=nil{fmt.Fprintln(os.Stderr,"test broker connection failed");os.Exit(1)};defer client.Disconnect(100)
	body,err:=io.ReadAll(io.LimitReader(os.Stdin,1<<20));if err!=nil{panic(err)}
	token=client.Publish(os.Args[1],1,false,body);if !token.WaitTimeout(10*time.Second)||token.Error()!=nil{fmt.Fprintln(os.Stderr,"test MQTT publish failed");os.Exit(1)}
}
