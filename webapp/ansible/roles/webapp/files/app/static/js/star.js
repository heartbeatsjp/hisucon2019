$(document).ready(function() {
    $('#star-button').click(function(event){
        $.ajax({
            data : $(this).parent('form').serialize(),
            type : 'POST',
            url : $(this).parent('form').attr('action')
        }).done(function(data){
            $('#star').text(data.output);
        });
        event.preventDefault();
    });
    $('#comment-star-button').click(function(event){
        $.ajax({
            data : $(this).parent('form').serialize(),
            type : 'POST',
            url : $(this).parent('form').attr('action')
        }).done(function(data){
            $('#comment-star').text(data.output);
        });
        event.preventDefault();
    });
});
