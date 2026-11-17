package io.github.scotylucas.kinora;

import android.os.Bundle;
import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {
    @Override
    public void onCreate(Bundle savedInstanceState) {
        registerPlugin(KinoraNativePlugin.class);
        super.onCreate(savedInstanceState);
    }
}
