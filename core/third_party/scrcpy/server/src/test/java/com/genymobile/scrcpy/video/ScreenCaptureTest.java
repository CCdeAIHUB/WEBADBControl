package com.genymobile.scrcpy.video;

import org.junit.Assert;
import org.junit.Test;

public class ScreenCaptureTest {

    @Test
    public void testPreferSurfaceControlForAndroid16XiaomiFamily() {
        Assert.assertTrue(ScreenCapture.shouldPreferSurfaceControl(36, "Xiaomi", "Xiaomi"));
        Assert.assertTrue(ScreenCapture.shouldPreferSurfaceControl(36, "Xiaomi Communications Co., Ltd.", "Redmi"));
        Assert.assertTrue(ScreenCapture.shouldPreferSurfaceControl(36, "unknown", "POCO"));
    }

    @Test
    public void testKeepDisplayManagerForOlderOrOtherDevices() {
        Assert.assertFalse(ScreenCapture.shouldPreferSurfaceControl(35, "Xiaomi", "Redmi"));
        Assert.assertFalse(ScreenCapture.shouldPreferSurfaceControl(36, "Google", "google"));
        Assert.assertFalse(ScreenCapture.shouldPreferSurfaceControl(36, null, null));
    }
}
