# openoutreach
Cold Email Messaging Tool With Pixel Tracking and Link Replacement // Lemlist Clone uses GMAIL 

Lemlist Alternative Using Golang With Pixel Tracking



First Add Your Google Cloud Credentials to the credentials folder at credentials.json


then use A Service like ngrok or telebit if you want to run on your own pc 


then add the tracking base url to the .env 


this allows you to track opens and link clicks (not perfect needs more work)


you can use the provided typescript client to easily interact with it // or use the frontend 



how it works on a very high level 

we have an execution chron and a stats chron 

when you create a campaign you upload leads and email sequence with time offset between them

when you start the campaign the first email is sequenced for all the leads and then every time an email is sent the next follow up gets sequenced this is to make sure that if someone replies they are removed from the flow

the execution chron checks if there is any task to execute

the sats chron checks all threads for replies to update replies

