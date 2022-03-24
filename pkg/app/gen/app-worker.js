const cacheName = "app-" + "{{.Version}}";

self.addEventListener("install", event => {
  console.log("installing app worker {{.Version}}");

  event.waitUntil(
    caches.open(cacheName).
      then(cache => {
        return cache.addAll([
          {{range $path, $element := .ResourcesToCache}}"{{$path}}",
          {{end}}
        ]);
      }).
      then(() => {
        self.skipWaiting();
      })
  );
});

self.addEventListener("activate", event => {
  event.waitUntil(
    caches.keys().then(keyList => {
      return Promise.all(
        keyList.map(key => {
          if (key !== cacheName) {
            return caches.delete(key);
          }
        })
      );
    })
  );
  console.log("app worker {{.Version}} is activated");
});

self.addEventListener("fetch", event => {
  event.respondWith(
    caches.match(event.request).then(response => {
      return response || fetch(event.request);
    })
  );
});

self.addEventListener('notificationclick', function(e) {
    // removes the notification from browser/system
    var action = e.action;
    e.notification.close();
    console.log("notificationclick: "+action)

    // Focus tab if open
    e.waitUntil(clients.matchAll({
        includeUncontrolled: true,
        type: 'window'
    }).then(function (clientList) {
        console.log("clients:" + clientList.length);
        for (var i = 0; i < clientList.length; ++i) {
            var client = clientList[i];
            if ('focus' in client) {
                // we may need the app to navigate
                client.postMessage({name: "notification", data: action, kind: "focused"});
                return client.focus()
            }
        }
        if (clients.openWindow) {
            // we open a new tab/window with the app
            return clients.openWindow('/').then(
                function(client) {
                    client.postMessage({name: "notification", data: action, kind: "opened"});
                }
            );
        }
    }));
});

self.addEventListener("push", event => {
    console.log("push received: " + event.data.text());
    self.clients.matchAll().then(function(clients) {
        clients.forEach(function (client) {
            client.postMessage({name: "push", data: event.data.text()});
        });
    });
});

self.addEventListener('message', function handler(event) {
    const title = 'Some Notification';
    var options = {
        body: event.text,
        //icon: 'images/example.png',
        vibrate: [100, 50, 100],
        data: {
            dateOfArrival: Date.now(),
            primaryKey: 1
        },
        actions: [
            {
                action: 'doit', title: 'Do this!',
                icon: 'images/checkmark.png'
            },
            {
                action: 'close', title: 'Close notification',
                icon: 'images/xmark.png'
            },
        ]
    };
    event.waitUntil(self.registration.showNotification(title, options));
    //console.log(event)
});