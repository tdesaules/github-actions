use std::env;

#[link(name = "gtk-4")]
extern "C" {
    fn gtk_init();
    fn gtk_window_new() -> *mut std::ffi::c_void;
    fn gtk_window_present(window: *mut std::ffi::c_void);
    fn gtk_window_destroy(window: *mut std::ffi::c_void);
    fn gtk_widget_get_mapped(widget: *mut std::ffi::c_void) -> i32;
}
#[link(name = "glib-2.0")]
extern "C" {
    fn g_main_context_iteration(context: *mut std::ffi::c_void, block: i32) -> i32;
}

fn main() {
    match env::args().nth(1).as_deref() {
        Some("--version") => println!(
            "niri-tonic-files {}",
            option_env!("NIRI_TONIC_RELEASE_VERSION").unwrap_or(env!("CARGO_PKG_VERSION"))
        ),
        Some("--check-config") => assert!(env::var_os("HOME").is_some()),
        Some("--smoke-test") => unsafe {
            gtk_init();
            let window = gtk_window_new();
            gtk_window_present(window);
            for _ in 0..100 {
                g_main_context_iteration(std::ptr::null_mut(), 0);
                if gtk_widget_get_mapped(window) != 0 {
                    gtk_window_destroy(window);
                    return;
                }
                std::thread::sleep(std::time::Duration::from_millis(10));
            }
            panic!("fixture GTK window did not map");
        },
        _ => panic!("unsupported fixture argument"),
    }
}

#[test]
fn fixture_version_is_defined() {
    assert_eq!(env!("CARGO_PKG_VERSION"), "0.1.0");
}
