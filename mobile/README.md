mobile
======

This package is intended to be built using [gomobile](https://github.com/golang/mobile). Corresponding targets were added to Makefile to build iOS framework (`make statusgo-ios-gomobile`) and Android library (`make statusgo-android-gomobile`).


## Usage iOS

Together with React Native, it can be used like this:

```obj-c
// Statusgo.h
#import <React/RCTBridgeModule.h>
#import <React/RCTEventEmitter.h>
#import <Status/Status.h>

@interface Statusd : RCTEventEmitter <RCTBridgeModule, StatusgoEventBus>

@property (nonatomic, strong) StatusAPI *status;

@end


// Statusgo.m
#import "Statusgo.h"
#import <React/RCTLog.h>
#import <Status/Status.h>

@implementation Statusgo

RCT_EXPORT_MODULE()

- (id)init {
  if (self = [super init]) {
    self.status = StatusNewAPI(self);
    return self;
  }

  return nil;
}

#pragma mark - RCTBridgeModule

- (NSArray<NSString *> *)supportedEvents {
  return @[EventStatus];
}

#pragma mark - RCTEventEmitter

- (void)sendEvent:(StatusgoEvent*)event {
  RCTLogInfo(@"Received an event: type=%ld", event.type);
  [self sendEventWithName:EventStatus body:event];
}

RCT_EXPORT_METHOD(startNodeAsync:(NSString*)config callback:(RCTResponseSenderBlock)callback) {
  NSError *error;
  [self.status startNodeAsync:config error:&error];

  if (error != nil) {
    RCTLogError(@"Error while starting a node: %@", error);
    callback(@[error]);
    return;
  }

  callback(@[[NSNull null]]);
}
```

## Usage Android

Coming soon.
