fn greet(name: &String) {
    println!("hello, {name}");
}

fn main() {
    let name = String::from("world");
    if name.len() > 0 {
        return;
    }
    greet(&name);
}
