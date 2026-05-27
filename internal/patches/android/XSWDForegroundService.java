package org.golang.app;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.Service;
import android.content.Intent;
import android.os.Build;
import android.os.IBinder;
import android.util.Log;

public class XSWDForegroundService extends Service {
    private static final int NOTIFICATION_ID = 1;
    private static final String CHANNEL_ID = "xswd_service_channel";

    @Override
    public void onCreate() {
        super.onCreate();
    }

	@Override
	public int onStartCommand(Intent intent, int flags, int startId) {
		String title = intent != null ? intent.getStringExtra("notification_title") : "Engram";
		String text = intent != null ? intent.getStringExtra("notification_text") : "XSWD server running";
		String channelName = intent != null ? intent.getStringExtra("channel_name") : "XSWD Service";
		String channelDesc = intent != null ? intent.getStringExtra("channel_description") : "Keeps Engram alive for XSWD WebSocket connections";

		createNotificationChannel(channelName, channelDesc);

		Notification.Builder builder;
		if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
			builder = new Notification.Builder(this, CHANNEL_ID);
		} else {
			builder = new Notification.Builder(this);
		}

		Notification notification = builder
				.setContentTitle(title)
				.setContentText(text)
				.setSmallIcon(android.R.drawable.ic_dialog_info)
				.setOngoing(true)
				.build();

		startForeground(NOTIFICATION_ID, notification);

		Log.d("Fyne", "XSWD foreground service started");
		return START_STICKY;
	}

    @Override
    public void onDestroy() {
        Log.d("Fyne", "XSWD foreground service stopping");
        stopForeground(true);
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

	private void createNotificationChannel(String channelName, String channelDesc) {
		if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
			NotificationChannel channel = new NotificationChannel(
					CHANNEL_ID,
					channelName != null ? channelName : "XSWD Service",
					NotificationManager.IMPORTANCE_LOW
			);
			channel.setDescription(channelDesc != null ? channelDesc : "Keeps Engram alive for XSWD WebSocket connections");
			NotificationManager manager = getSystemService(NotificationManager.class);
			manager.createNotificationChannel(channel);
		}
	}
}
