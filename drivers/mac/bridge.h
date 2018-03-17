#ifndef bridge_h
#define bridge_h

#import <Cocoa/Cocoa.h>

// GoBridge is a golang bridge implementation.
@interface GoBridge : NSObject
- (void)request:(NSString *)path payload:(NSString *)payload;
- (NSString *)requestWithResult:(NSString *)path payload:(NSString *)payload;
@end

void macCall(char *rawCall);
void defer(NSString *returnID, dispatch_block_t block);

typedef void (^MacRPCHandler)(id, NSString *);

@interface MacRPC : NSObject
@property NSMutableDictionary<NSString *, MacRPCHandler> *handlers;

- (instancetype)init;
- (void)handle:(NSString *)method withHandler:(MacRPCHandler)handler;
- (void) return:(NSString *)returnID
     withOutput:(id)out
       andError:(NSString *)err;
@end

@interface GoRPC : NSObject
- (id)call:(NSString *)method withInput:(id)in;
@end

#endif /* bridge_h */
