An attempt to try websocket using the default Go library.

To replicate this, open 3 terminals:

Terminal 1 for the server.
```bash
./socket_testing/server/run.sh
```

Terminal 2 and 3 for the client.
```bash
# Terminal 2
./socket_testing/client/run.sh
# The terminal looks like it's hanging, but it's not. It's waiting for user's input. Try typing something then press Enter.
```
```bash
# Terminal 3
./socket_testing/client/run.sh
# The terminal looks like it's hanging, but it's not. It's waiting for user's input. Try typing something then press Enter.
```

The server will:
- Register incoming user to the user list.
- Remove disconnected user from the user list.
- When receive a message from one user, the message will be broadcasted to the rest of the users.
- There's no user's name unique checking yet, so if there are 2 users with the same endpoint, they will be blind to each other's messages. And I don't bother implement such feature for this testing.

The client will:
- Wait for messages from the server, and print it.
- Send message to the server by inputing. 
